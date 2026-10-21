package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// ---------------- фоновые задания (установка сервера) ----------------

type Job struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	Done    bool
	OK      bool
	Error   string
	started time.Time
}

func (j *Job) Write(p []byte) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.buf.Write(p)
}

type Jobs struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

func NewJobs() *Jobs { return &Jobs{jobs: map[string]*Job{}} }

func (js *Jobs) New() (string, *Job) {
	id := newID()
	j := &Job{started: time.Now()}
	js.mu.Lock()
	js.jobs[id] = j
	js.mu.Unlock()
	return id, j
}

func (js *Jobs) Get(id string) *Job {
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.jobs[id]
}

func (js *Jobs) Cleanup() {
	js.mu.Lock()
	defer js.mu.Unlock()
	for id, j := range js.jobs {
		if time.Since(j.started) > 2*time.Hour {
			delete(js.jobs, id)
		}
	}
}

// ---------------- общие помощники ----------------

type apiError struct {
	code int
	msg  string
}

func (e apiError) Error() string { return e.msg }

func bad(msg string) error      { return apiError{http.StatusBadRequest, msg} }
func notFound(msg string) error { return apiError{http.StatusNotFound, msg} }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return bad("некорректный запрос")
	}
	return nil
}

type handler func(w http.ResponseWriter, r *http.Request) (any, error)

// wrap: JSON-ответы, проверка сессии и защита от CSRF (заголовок X-Requested-With).
func (a *App) wrap(public bool, h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Header.Get("X-Requested-With") != "ttpanel" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "запрос отклонён"})
			return
		}
		if !public && a.auth.User(r) == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "нужно войти"})
			return
		}
		res, err := h(w, r)
		if err != nil {
			var ae apiError
			code := http.StatusInternalServerError
			if errors.As(err, &ae) {
				code = ae.code
			} else {
				log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
			}
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		if res == nil {
			res = map[string]bool{"ok": true}
		}
		if res != skip {
			writeJSON(w, http.StatusOK, res)
		}
	}
}

var skip = &struct{}{}

func (a *App) routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/session", a.wrap(true, a.hSession))
	m.HandleFunc("POST /api/setup", a.wrap(true, a.hSetup))
	m.HandleFunc("POST /api/login", a.wrap(true, a.hLogin))
	m.HandleFunc("POST /api/logout", a.wrap(true, a.hLogout))

	m.HandleFunc("GET /api/state", a.wrap(false, a.hState))
	m.HandleFunc("POST /api/servers", a.wrap(false, a.hAddServer))
	m.HandleFunc("PUT /api/servers/{id}", a.wrap(false, a.hEditServer))
	m.HandleFunc("DELETE /api/servers/{id}", a.wrap(false, a.hDeleteServer))
	m.HandleFunc("POST /api/servers/{id}/refresh", a.wrap(false, a.hRefresh))
	m.HandleFunc("POST /api/servers/{id}/import", a.wrap(false, a.hImport))
	m.HandleFunc("POST /api/servers/{id}/install", a.wrap(false, a.hInstall))
	m.HandleFunc("POST /api/servers/{id}/service", a.wrap(false, a.hService))
	m.HandleFunc("GET /api/servers/{id}/logs", a.wrap(false, a.hLogs))
	m.HandleFunc("POST /api/servers/{id}/metrics", a.wrap(false, a.hEnableMetrics))
	m.HandleFunc("POST /api/servers/{id}/lan", a.wrap(false, a.hLanAccess))
	m.HandleFunc("GET /api/jobs/{id}", a.wrap(false, a.hJob))

	m.HandleFunc("POST /api/clients", a.wrap(false, a.hAddClient))
	m.HandleFunc("PUT /api/clients/{id}", a.wrap(false, a.hEditClient))
	m.HandleFunc("DELETE /api/clients/{id}", a.wrap(false, a.hDeleteClient))
	m.HandleFunc("GET /api/clients/{id}/link", a.wrap(false, a.hClientLink))
	m.HandleFunc("GET /api/clients/{id}/qr.png", a.wrap(false, a.hClientQR))
	m.HandleFunc("GET /api/clients/{id}/config.toml", a.wrap(false, a.hClientToml))

	m.HandleFunc("GET /api/settings", a.wrap(false, a.hSettings))
	m.HandleFunc("POST /api/settings/password", a.wrap(false, a.hPassword))
	m.HandleFunc("GET /api/backup", a.wrap(false, a.hBackup))
}

// ---------------- вход ----------------

func (a *App) hasAdmin() bool {
	has := false
	a.store.View(func(d *Data) { has = d.Settings.PassHash != "" })
	return has
}

func (a *App) hSession(w http.ResponseWriter, r *http.Request) (any, error) {
	return map[string]any{"setup": !a.hasAdmin(), "user": a.auth.User(r), "version": version}, nil
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *App) hSetup(w http.ResponseWriter, r *http.Request) (any, error) {
	var req loginReq
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 8 {
		return nil, bad("укажите логин и пароль не короче 8 символов")
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	err = a.store.Update(func(d *Data) error {
		if d.Settings.PassHash != "" {
			return apiError{http.StatusConflict, "администратор уже создан"}
		}
		d.Settings = Settings{AdminUser: req.Username, PassHash: hash}
		return nil
	})
	if err != nil {
		return nil, err
	}
	a.auth.Start(w, r, req.Username)
	return nil, nil
}

func (a *App) hLogin(w http.ResponseWriter, r *http.Request) (any, error) {
	ip := clientIP(r)
	if a.auth.Blocked(ip) {
		return nil, apiError{http.StatusTooManyRequests, "слишком много попыток — подождите 10 минут"}
	}
	var req loginReq
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	var st Settings
	a.store.View(func(d *Data) { st = d.Settings })
	if st.PassHash == "" || req.Username != st.AdminUser || !checkPassword(st.PassHash, req.Password) {
		a.auth.Fail(ip)
		time.Sleep(400 * time.Millisecond)
		return nil, apiError{http.StatusUnauthorized, "неверный логин или пароль"}
	}
	a.auth.Start(w, r, st.AdminUser)
	return nil, nil
}

func (a *App) hLogout(w http.ResponseWriter, r *http.Request) (any, error) {
	a.auth.End(w, r)
	return nil, nil
}

// ---------------- состояние ----------------

type serverView struct {
	Server
	Status *ServerStatus `json:"status"`
}

func (a *App) hState(w http.ResponseWriter, r *http.Request) (any, error) {
	a.stats.Touch()
	var servers []serverView
	var clients []Client
	a.store.View(func(d *Data) {
		for _, s := range d.Servers {
			sv := serverView{Server: *s}
			sv.HostKey = ""
			servers = append(servers, sv)
		}
		for _, c := range d.Clients {
			cc := *c
			cc.Password = ""
			clients = append(clients, cc)
		}
	})
	a.stMu.Lock()
	for i := range servers {
		if st, ok := a.status[servers[i].ID]; ok {
			stc := st
			servers[i].Status = &stc
		}
	}
	a.stMu.Unlock()
	sort.SliceStable(clients, func(i, j int) bool { return clients[i].CreatedAt.Before(clients[j].CreatedAt) })
	if servers == nil {
		servers = []serverView{}
	}
	if clients == nil {
		clients = []Client{}
	}
	live, statsErrors := a.stats.Snapshot()
	return map[string]any{"servers": servers, "clients": clients, "live": live, "statsErrors": statsErrors, "now": time.Now()}, nil
}

func (a *App) hEnableMetrics(w http.ResponseWriter, r *http.Request) (any, error) {
	s, err := a.server(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	l := a.ssh.ServerLock(s.ID)
	l.Lock()
	err = a.enableMetrics(&s)
	l.Unlock()
	if err != nil {
		return nil, err
	}
	// служба перезапустится через 2 с — обновим статус чуть позже
	go func() {
		time.Sleep(5 * time.Second)
		a.refreshOne(&s)
	}()
	return nil, nil
}

func (a *App) hLanAccess(w http.ResponseWriter, r *http.Request) (any, error) {
	s, err := a.server(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	l := a.ssh.ServerLock(s.ID)
	l.Lock()
	err = a.setLanAccess(&s, req.Enabled)
	l.Unlock()
	if err != nil {
		return nil, err
	}
	// сразу отражаем новое значение, полный опрос — после перезапуска
	a.stMu.Lock()
	if st, ok := a.status[s.ID]; ok {
		st.LanAccess = req.Enabled
		a.status[s.ID] = st
	}
	a.stMu.Unlock()
	go func() {
		time.Sleep(5 * time.Second)
		a.refreshOne(&s)
	}()
	return nil, nil
}

// ---------------- серверы ----------------

type serverReq struct {
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	User          string `json:"user"`
	Password      string `json:"password"`
	PublicAddress string `json:"publicAddress"`
}

func (q *serverReq) normalize() error {
	q.Name = strings.TrimSpace(q.Name)
	q.Host = strings.TrimSpace(q.Host)
	q.User = strings.TrimSpace(q.User)
	q.PublicAddress = strings.TrimSpace(q.PublicAddress)
	if q.Port == 0 {
		q.Port = 22
	}
	if q.User == "" {
		q.User = "root"
	}
	if !reHost.MatchString(q.Host) {
		return bad("укажите IP-адрес или домен сервера")
	}
	if q.Port < 1 || q.Port > 65535 {
		return bad("неверный порт SSH")
	}
	if q.PublicAddress != "" && !reHost.MatchString(q.PublicAddress) {
		return bad("неверный адрес для клиентов")
	}
	if q.Name == "" {
		q.Name = q.Host
	}
	if len(q.Name) > 64 {
		return bad("слишком длинное название")
	}
	return nil
}

func (a *App) server(id string) (Server, error) {
	var s Server
	found := false
	a.store.View(func(d *Data) {
		if p := d.Server(id); p != nil {
			s = *p
			found = true
		}
	})
	if !found {
		return s, notFound("сервер не найден")
	}
	return s, nil
}

func (a *App) hAddServer(w http.ResponseWriter, r *http.Request) (any, error) {
	var req serverReq
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	if err := req.normalize(); err != nil {
		return nil, err
	}
	s := &Server{ID: newID(), Name: req.Name, Host: req.Host, Port: req.Port, User: req.User,
		PublicAddress: req.PublicAddress, InstallDir: defaultInstallDir, CreatedAt: time.Now()}
	if err := a.store.Update(func(d *Data) error { d.Servers = append(d.Servers, s); return nil }); err != nil {
		return nil, err
	}
	fail := func(err error) (any, error) {
		a.store.Update(func(d *Data) error {
			for i, x := range d.Servers {
				if x.ID == s.ID {
					d.Servers = append(d.Servers[:i], d.Servers[i+1:]...)
				}
			}
			return nil
		})
		return nil, err
	}
	// Ключ панели ставится по паролю один раз; дальше — только по ключу.
	if req.Password != "" {
		if err := a.ssh.InstallKey(s, req.Password); err != nil {
			return fail(bad("не удалось войти по паролю: " + err.Error()))
		}
	}
	fresh, _ := a.server(s.ID)
	st := a.refreshOne(&fresh)
	if !st.SSHOK {
		return fail(bad("нет доступа по SSH-ключу панели. Введите пароль root или добавьте ключ панели на сервер (Настройки). " + st.Error))
	}
	imported := 0
	if st.Installed {
		n, err := a.importClients(&fresh)
		if err == nil {
			imported = n
		}
	}
	return map[string]any{"id": s.ID, "installed": st.Installed, "imported": imported}, nil
}

func (a *App) hEditServer(w http.ResponseWriter, r *http.Request) (any, error) {
	var req serverReq
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	if err := req.normalize(); err != nil {
		return nil, err
	}
	id := r.PathValue("id")
	err := a.store.Update(func(d *Data) error {
		s := d.Server(id)
		if s == nil {
			return notFound("сервер не найден")
		}
		if s.Host != req.Host || s.Port != req.Port {
			s.HostKey = "" // другой адрес — запомним ключ заново
		}
		s.Name, s.Host, s.Port, s.User, s.PublicAddress = req.Name, req.Host, req.Port, req.User, req.PublicAddress
		return nil
	})
	a.ssh.Drop(id)
	return nil, err
}

func (a *App) hDeleteServer(w http.ResponseWriter, r *http.Request) (any, error) {
	id := r.PathValue("id")
	a.ssh.Drop(id)
	err := a.store.Update(func(d *Data) error {
		for i, s := range d.Servers {
			if s.ID == id {
				d.Servers = append(d.Servers[:i], d.Servers[i+1:]...)
				var keep []*Client
				for _, c := range d.Clients {
					if c.ServerID != id {
						keep = append(keep, c)
					}
				}
				d.Clients = keep
				return nil
			}
		}
		return notFound("сервер не найден")
	})
	a.stMu.Lock()
	delete(a.status, id)
	a.stMu.Unlock()
	return nil, err
}

func (a *App) hRefresh(w http.ResponseWriter, r *http.Request) (any, error) {
	s, err := a.server(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	st := a.refreshOne(&s)
	return st, nil
}

// importClients подтягивает пользователей из credentials.toml в панель (существующие не трогает).
func (a *App) importClients(s *Server) (int, error) {
	creds, err := a.readCredentials(s)
	if err != nil {
		return 0, err
	}
	n := 0
	err = a.store.Update(func(d *Data) error {
		existing := map[string]*Client{}
		for _, c := range d.ServerClients(s.ID) {
			existing[c.Username] = c
		}
		for _, cr := range creds {
			if cr.Username == "" {
				continue
			}
			if c, ok := existing[cr.Username]; ok {
				c.Password = cr.Password
				c.Enabled = true
				continue
			}
			d.Clients = append(d.Clients, &Client{ID: newID(), ServerID: s.ID, Username: cr.Username, Password: cr.Password,
				Enabled: true, Note: "импортирован с сервера", CreatedAt: time.Now()})
			n++
		}
		return nil
	})
	return n, err
}

func (a *App) hImport(w http.ResponseWriter, r *http.Request) (any, error) {
	s, err := a.server(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	n, err := a.importClients(&s)
	if err != nil {
		return nil, err
	}
	return map[string]int{"imported": n}, nil
}

func (a *App) hInstall(w http.ResponseWriter, r *http.Request) (any, error) {
	s, err := a.server(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	var p InstallParams
	if err := readJSON(r, &p); err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, bad(err.Error())
	}
	a.stMu.Lock()
	st := a.status[s.ID]
	a.stMu.Unlock()
	if st.Installed {
		return nil, bad("TrustTunnel уже установлен на этом сервере")
	}
	password := randHex(16)
	id, job := a.jobs.New()
	go func() {
		l := a.ssh.ServerLock(s.ID)
		l.Lock()
		defer l.Unlock()
		err := a.ssh.Stream(&s, installScript(&s, p, password), 15*time.Minute, job)
		job.mu.Lock()
		job.Done = true
		job.OK = err == nil
		if err != nil {
			job.Error = err.Error()
		}
		job.mu.Unlock()
		if err == nil {
			a.store.Update(func(d *Data) error {
				d.Clients = append(d.Clients, &Client{ID: newID(), ServerID: s.ID, Username: p.FirstClient, Password: password,
					Enabled: true, Note: "создан при установке", CreatedAt: time.Now()})
				if srv := d.Server(s.ID); srv != nil && srv.PublicAddress == "" && p.Port != 443 {
					srv.PublicAddress = fmt.Sprintf("%s:%d", s.Host, p.Port)
				}
				return nil
			})
		}
		a.refreshOne(&s)
	}()
	return map[string]string{"job": id}, nil
}

func (a *App) hJob(w http.ResponseWriter, r *http.Request) (any, error) {
	j := a.jobs.Get(r.PathValue("id"))
	if j == nil {
		return nil, notFound("задание не найдено")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return map[string]any{"done": j.Done, "ok": j.OK, "error": j.Error, "log": j.buf.String()}, nil
}

func (a *App) hService(w http.ResponseWriter, r *http.Request) (any, error) {
	s, err := a.server(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	var req struct {
		Action string `json:"action"`
	}
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	l := a.ssh.ServerLock(s.ID)
	l.Lock()
	out, err := a.serviceAction(&s, req.Action)
	l.Unlock()
	if err != nil {
		return nil, err
	}
	st := a.refreshOne(&s)
	return map[string]any{"output": out, "status": st}, nil
}

func (a *App) hLogs(w http.ResponseWriter, r *http.Request) (any, error) {
	s, err := a.server(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	out, err := a.serviceLogs(&s, 300)
	if err != nil {
		return nil, err
	}
	return map[string]string{"log": out}, nil
}

// ---------------- клиенты ----------------

type clientReq struct {
	ServerID  string  `json:"serverId"`
	Username  string  `json:"username"`
	Note      string  `json:"note"`
	ExpiresAt *string `json:"expiresAt"` // YYYY-MM-DD или пусто
	Enabled   *bool   `json:"enabled"`
}

func parseExpiry(v *string) (*time.Time, bool, error) {
	if v == nil {
		return nil, false, nil
	}
	if *v == "" {
		return nil, true, nil
	}
	t, err := time.ParseInLocation("2006-01-02", *v, time.Local)
	if err != nil {
		return nil, false, bad("неверная дата")
	}
	t = t.Add(24*time.Hour - time.Second) // действует до конца дня
	return &t, true, nil
}

func (a *App) hAddClient(w http.ResponseWriter, r *http.Request) (any, error) {
	var req clientReq
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	req.Username = strings.TrimSpace(req.Username)
	if !reUsername.MatchString(req.Username) {
		return nil, bad("имя: латиница, цифры, точка, дефис или подчёркивание, до 32 символов")
	}
	exp, _, err := parseExpiry(req.ExpiresAt)
	if err != nil {
		return nil, err
	}
	c := &Client{ID: newID(), ServerID: req.ServerID, Username: req.Username, Password: randHex(16),
		Enabled: true, Note: strings.TrimSpace(req.Note), ExpiresAt: exp, CreatedAt: time.Now()}
	err = a.store.Update(func(d *Data) error {
		if d.Server(req.ServerID) == nil {
			return notFound("сервер не найден")
		}
		for _, x := range d.ServerClients(req.ServerID) {
			if strings.EqualFold(x.Username, c.Username) {
				return bad("клиент с таким именем уже есть на этом сервере")
			}
		}
		d.Clients = append(d.Clients, c)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := a.syncServer(req.ServerID); err != nil {
		a.store.Update(func(d *Data) error {
			for i, x := range d.Clients {
				if x.ID == c.ID {
					d.Clients = append(d.Clients[:i], d.Clients[i+1:]...)
				}
			}
			return nil
		})
		return nil, err
	}
	return map[string]string{"id": c.ID}, nil
}

func (a *App) hEditClient(w http.ResponseWriter, r *http.Request) (any, error) {
	var req clientReq
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	exp, expSet, err := parseExpiry(req.ExpiresAt)
	if err != nil {
		return nil, err
	}
	id := r.PathValue("id")
	var serverID string
	needSync := false
	var prevEnabled bool
	err = a.store.Update(func(d *Data) error {
		c := d.Client(id)
		if c == nil {
			return notFound("клиент не найден")
		}
		serverID = c.ServerID
		prevEnabled = c.Enabled
		c.Note = strings.TrimSpace(req.Note)
		if expSet {
			c.ExpiresAt = exp
		}
		if req.Enabled != nil && *req.Enabled != c.Enabled {
			if *req.Enabled && c.ExpiresAt != nil && time.Now().After(*c.ExpiresAt) {
				return bad("срок действия истёк — сначала продлите его")
			}
			c.Enabled = *req.Enabled
			needSync = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if needSync {
		if err := a.syncServer(serverID); err != nil {
			a.store.Update(func(d *Data) error {
				if c := d.Client(id); c != nil {
					c.Enabled = prevEnabled
				}
				return nil
			})
			return nil, err
		}
	}
	return nil, nil
}

func (a *App) hDeleteClient(w http.ResponseWriter, r *http.Request) (any, error) {
	id := r.PathValue("id")
	var removed *Client
	err := a.store.Update(func(d *Data) error {
		for i, c := range d.Clients {
			if c.ID == id {
				removed = c
				d.Clients = append(d.Clients[:i], d.Clients[i+1:]...)
				return nil
			}
		}
		return notFound("клиент не найден")
	})
	if err != nil {
		return nil, err
	}
	if removed.Enabled {
		if err := a.syncServer(removed.ServerID); err != nil {
			a.store.Update(func(d *Data) error { d.Clients = append(d.Clients, removed); return nil })
			return nil, err
		}
	}
	return nil, nil
}

func (a *App) clientAndServer(id string) (Client, Server, error) {
	var c Client
	var s Server
	ok := false
	a.store.View(func(d *Data) {
		if p := d.Client(id); p != nil {
			c = *p
			if sp := d.Server(p.ServerID); sp != nil {
				s = *sp
				ok = true
			}
		}
	})
	if !ok {
		return c, s, notFound("клиент не найден")
	}
	if !c.Enabled {
		return c, s, bad("клиент отключён — включите его, чтобы получить конфиг")
	}
	return c, s, nil
}

func (a *App) hClientLink(w http.ResponseWriter, r *http.Request) (any, error) {
	c, s, err := a.clientAndServer(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	link, err := a.exportClient(&s, c.Username, "deeplink")
	if err != nil {
		return nil, err
	}
	return map[string]string{"link": link}, nil
}

func (a *App) hClientQR(w http.ResponseWriter, r *http.Request) (any, error) {
	c, s, err := a.clientAndServer(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	link, err := a.exportClient(&s, c.Username, "deeplink")
	if err != nil {
		return nil, err
	}
	level := qrcode.Medium
	if len(link) > 600 {
		level = qrcode.Low // ссылки с самоподписанным сертификатом длинные
	}
	png, err := qrcode.Encode(link, level, 640)
	if err != nil {
		return nil, err
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(png)
	return skip, nil
}

func (a *App) hClientToml(w http.ResponseWriter, r *http.Request) (any, error) {
	c, s, err := a.clientAndServer(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	cfg, err := a.exportClient(&s, c.Username, "toml")
	if err != nil {
		return nil, err
	}
	w.Header().Set("Content-Type", "application/toml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.toml"`, safeName(s.Name), c.Username))
	w.Write([]byte(cfg))
	return skip, nil
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// ---------------- настройки ----------------

func (a *App) hSettings(w http.ResponseWriter, r *http.Request) (any, error) {
	var user string
	a.store.View(func(d *Data) { user = d.Settings.AdminUser })
	return map[string]string{"user": user, "publicKey": a.ssh.PublicKey, "version": version}, nil
}

func (a *App) hPassword(w http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	if len(req.New) < 8 {
		return nil, bad("новый пароль должен быть не короче 8 символов")
	}
	var st Settings
	a.store.View(func(d *Data) { st = d.Settings })
	if !checkPassword(st.PassHash, req.Current) {
		return nil, bad("текущий пароль неверен")
	}
	hash, err := hashPassword(req.New)
	if err != nil {
		return nil, err
	}
	if err := a.store.Update(func(d *Data) error { d.Settings.PassHash = hash; return nil }); err != nil {
		return nil, err
	}
	a.auth.EndAll()
	a.auth.Start(w, r, st.AdminUser)
	return nil, nil
}

func (a *App) hBackup(w http.ResponseWriter, r *http.Request) (any, error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="ttpanel-backup-`+time.Now().Format("2006-01-02")+`.json"`)
	w.Write(a.store.Raw())
	return skip, nil
}
