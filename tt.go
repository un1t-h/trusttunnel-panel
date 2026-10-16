package main

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Операции с TrustTunnel на удалённом сервере (всё через SSH, без агентов).

const defaultInstallDir = "/opt/trusttunnel"

var (
	reUsername = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,32}$`)
	reHost     = regexp.MustCompile(`^[A-Za-z0-9.:\-\[\]]{1,253}$`)
	reEmail    = regexp.MustCompile(`^[^@\s'"]+@[^@\s'"]+\.[^@\s'"]+$`)
)

func dirOf(s *Server) string {
	if s.InstallDir == "" {
		return defaultInstallDir
	}
	return s.InstallDir
}

// Префикс скрипта: каталог установки и путь к credentials.toml из vpn.toml.
func ttPrelude(s *Server) string {
	return "D=" + shq(dirOf(s)) + "; cd \"$D\" 2>/dev/null; " +
		"CF=$(sed -n 's/^[[:space:]]*credentials_file[[:space:]]*=[[:space:]]*\"\\(.*\\)\".*/\\1/p' vpn.toml 2>/dev/null | head -1); " +
		"[ -z \"$CF\" ] && CF=credentials.toml; case \"$CF\" in /*) ;; *) CF=\"$D/$CF\";; esac; "
}

type ServerStatus struct {
	Reachable bool      `json:"reachable"`
	PingMs    int       `json:"pingMs"`
	SSHOK     bool      `json:"sshOk"`
	Installed bool      `json:"installed"`
	Active    string    `json:"active"` // active / inactive / failed / ...
	Version   string    `json:"version"`
	Load      string    `json:"load"`
	MemUsed   int64     `json:"memUsed"` // КБ
	MemTotal  int64     `json:"memTotal"`
	Uptime    int64     `json:"uptime"` // с
	Conns     int       `json:"conns"`
	Port      string    `json:"port"`
	Metrics   string    `json:"metrics"` // on — статистика клиентов включена; partial — метрики без per_client; off
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

func (a *App) probe(s *Server) ServerStatus {
	st := ServerStatus{CheckedAt: time.Now()}
	d, err := Ping(s)
	if err != nil {
		st.Error = "сервер недоступен: " + err.Error()
		return st
	}
	st.Reachable = true
	st.PingMs = int(d.Milliseconds())
	script := ttPrelude(s) + `
if [ -x "$D/trusttunnel_endpoint" ]; then echo installed=1; echo version=$("$D/trusttunnel_endpoint" --version 2>/dev/null | head -1); else echo installed=0; fi
echo active=$(systemctl is-active trusttunnel 2>/dev/null)
P=$(sed -n 's/^[[:space:]]*listen_address[[:space:]]*=.*:\([0-9]*\)".*/\1/p' vpn.toml 2>/dev/null | head -1); [ -z "$P" ] && P=443
echo port=$P
echo load=$(cut -d' ' -f1 /proc/loadavg)
awk '/MemTotal/{t=$2}/MemAvailable/{a=$2}END{print "mem="(t-a)"/"t}' /proc/meminfo
echo uptime=$(cut -d. -f1 /proc/uptime)
echo conns=$(ss -Htn state established "( sport = :$P )" 2>/dev/null | wc -l)
` + metricsAddr + `echo metrics=$(curl -s -m 2 -o /dev/null -w '%{http_code}' "http://$MA/clients" 2>/dev/null)
`
	out, _, err := a.ssh.Run(s, script, "", 15*time.Second)
	if err != nil {
		st.Error = "SSH: " + err.Error()
		return st
	}
	st.SSHOK = true
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch k {
		case "installed":
			st.Installed = v == "1"
		case "version":
			st.Version = strings.TrimSpace(v)
		case "active":
			st.Active = strings.TrimSpace(v)
		case "port":
			st.Port = v
		case "load":
			st.Load = v
		case "mem":
			u, t, _ := strings.Cut(v, "/")
			st.MemUsed, _ = strconv.ParseInt(u, 10, 64)
			st.MemTotal, _ = strconv.ParseInt(t, 10, 64)
		case "uptime":
			st.Uptime, _ = strconv.ParseInt(v, 10, 64)
		case "conns":
			st.Conns, _ = strconv.Atoi(strings.TrimSpace(v))
		case "metrics":
			switch strings.TrimSpace(v) {
			case "200":
				st.Metrics = "on"
			case "404":
				st.Metrics = "partial"
			default:
				st.Metrics = "off"
			}
		}
	}
	return st
}

type cred struct{ Username, Password string }

// readCredentials читает [[client]] из credentials.toml.
func (a *App) readCredentials(s *Server) ([]cred, error) {
	out, errOut, err := a.ssh.Run(s, ttPrelude(s)+`cat "$CF"`, "", 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать список пользователей: %v %s", err, strings.TrimSpace(errOut))
	}
	return parseCredentials(out), nil
}

func parseCredentials(text string) []cred {
	var res []cred
	var cur *cred
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "[[client]]" {
			res = append(res, cred{})
			cur = &res[len(res)-1]
			continue
		}
		if strings.HasPrefix(line, "[") {
			cur = nil
			continue
		}
		if cur == nil {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = tomlUnquote(strings.TrimSpace(v))
		switch k {
		case "username":
			cur.Username = v
		case "password":
			cur.Password = v
		}
	}
	return res
}

func tomlUnquote(v string) string {
	if i := strings.Index(v, " #"); i > 0 && (strings.HasPrefix(v, "'") || strings.HasPrefix(v, "\"")) {
		if j := strings.LastIndexAny(v[:i], "'\""); j > 0 {
			v = v[:j+1]
		}
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return v[1 : len(v)-1]
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		if u, err := strconv.Unquote(v); err == nil {
			return u
		}
		return v[1 : len(v)-1]
	}
	return v
}

func tomlQuote(s string) string { return strconv.Quote(s) }

// writeCredentials записывает включённых клиентов и перезапускает сервис
// (TrustTunnel перечитывает credentials.toml только при перезапуске).
func (a *App) writeCredentials(s *Server, clients []*Client) error {
	var b strings.Builder
	b.WriteString("# Управляется TrustTunnel Panel. Ручные правки будут перезаписаны.\n")
	for _, c := range clients {
		if !c.Enabled {
			continue
		}
		fmt.Fprintf(&b, "\n[[client]]\nusername = %s\npassword = %s\n", tomlQuote(c.Username), tomlQuote(c.Password))
	}
	// Перезапуск откладываем на 2 секунды и сразу отвечаем: если администратор сам сидит в панели
	// через этот VPN, ответ успевает дойти до того, как перезапуск оборвёт соединения.
	cmd := ttPrelude(s) + `[ -f "$CF" ] && cp -p "$CF" "$CF.bak"; umask 077; cat > "$CF.tmp" && mv "$CF.tmp" "$CF" || exit 1
systemd-run --quiet --on-active=2 --timer-property=AccuracySec=100ms systemctl restart trusttunnel 2>/dev/null ||
  (nohup sh -c 'sleep 2; systemctl restart trusttunnel' >/dev/null 2>&1 &)`
	out, errOut, err := a.ssh.Run(s, cmd, b.String(), 30*time.Second)
	if err != nil {
		return fmt.Errorf("не удалось применить изменения на сервере: %v %s %s", err, strings.TrimSpace(out), strings.TrimSpace(errOut))
	}
	return nil
}

// exportClient возвращает ссылку tt:// или TOML-конфиг клиента.
func (a *App) exportClient(s *Server, username, format string) (string, error) {
	address := s.PublicAddress
	if address == "" {
		address = s.Host
	}
	cmd := ttPrelude(s) + `./trusttunnel_endpoint vpn.toml hosts.toml -c ` + shq(username) + ` -a ` + shq(address) +
		` -n ` + shq(s.Name) + ` -f ` + shq(format)
	out, errOut, err := a.ssh.Run(s, cmd, "", 20*time.Second)
	if err != nil {
		return "", fmt.Errorf("сервер не выдал конфиг: %v %s", err, strings.TrimSpace(errOut))
	}
	if format == "deeplink" {
		for _, line := range strings.Split(out, "\n") {
			if i := strings.Index(line, "tt://"); i >= 0 {
				return strings.TrimSpace(line[i:]), nil
			}
		}
		return "", fmt.Errorf("в ответе сервера нет ссылки tt://")
	}
	return out, nil
}

func (a *App) serviceAction(s *Server, action string) (string, error) {
	switch action {
	case "restart", "start", "stop":
	default:
		return "", fmt.Errorf("неизвестное действие")
	}
	out, errOut, err := a.ssh.Run(s, "systemctl "+action+" trusttunnel; sleep 1; systemctl is-active trusttunnel", "", 40*time.Second)
	return strings.TrimSpace(out + errOut), ignoreExit(err)
}

func (a *App) serviceLogs(s *Server, lines int) (string, error) {
	out, errOut, err := a.ssh.Run(s, fmt.Sprintf("journalctl -u trusttunnel -n %d --no-pager -o short-iso 2>&1", lines), "", 20*time.Second)
	return out + errOut, ignoreExit(err)
}

func ignoreExit(err error) error {
	if err != nil && strings.Contains(err.Error(), "exited with status") {
		return nil
	}
	return err
}

type InstallParams struct {
	Hostname    string `json:"hostname"`
	Port        int    `json:"port"`
	CertType    string `json:"certType"` // self-signed | letsencrypt
	Email       string `json:"email"`
	FirstClient string `json:"firstClient"`
}

func (p *InstallParams) validate() error {
	if p.Port == 0 {
		p.Port = 443
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("неверный порт")
	}
	if p.Hostname == "" {
		p.Hostname = "vpn.tt.internal"
	}
	if !reHost.MatchString(p.Hostname) {
		return fmt.Errorf("неверное имя хоста")
	}
	if p.CertType != "letsencrypt" {
		p.CertType = "self-signed"
	} else if !reEmail.MatchString(p.Email) {
		return fmt.Errorf("для Let's Encrypt нужен e-mail")
	}
	if p.FirstClient == "" {
		p.FirstClient = "user1"
	}
	if !reUsername.MatchString(p.FirstClient) {
		return fmt.Errorf("имя клиента: латиница, цифры, точка, дефис, подчёркивание")
	}
	return nil
}

// installScript ставит официальный endpoint и настраивает его как systemd-сервис.
func installScript(s *Server, p InstallParams, password string) string {
	cert := "--cert-type self-signed"
	if p.CertType == "letsencrypt" {
		cert = "--cert-type letsencrypt --acme-email " + shq(p.Email)
	}
	return `set -e
if [ -f ` + shq(dirOf(s)+"/vpn.toml") + ` ]; then echo "TrustTunnel на этом сервере уже настроен — установка отменена, чтобы не затереть конфигурацию."; exit 3; fi
export DEBIAN_FRONTEND=noninteractive
command -v curl >/dev/null || (apt-get update -qq && apt-get install -y -qq curl)
echo "==> Скачиваю и устанавливаю TrustTunnel"
curl -fsSL https://raw.githubusercontent.com/TrustTunnel/TrustTunnel/refs/heads/master/scripts/install.sh | sh -s - -o ` + shq(dirOf(s)) + `
cd ` + shq(dirOf(s)) + `
echo "==> Генерирую конфигурацию"
./setup_wizard -m non-interactive -a 0.0.0.0:` + strconv.Itoa(p.Port) + ` -c ` + shq(p.FirstClient+":"+password) +
		` -n ` + shq(p.Hostname) + ` ` + cert + ` --lib-settings vpn.toml --hosts-settings hosts.toml
echo "==> Включаю статистику клиентов (только локально, 127.0.0.1)"
` + enableMetricsScript + `echo "==> Настраиваю systemd"
cp trusttunnel.service.template /etc/systemd/system/trusttunnel.service
systemctl daemon-reload
systemctl enable trusttunnel
systemctl restart trusttunnel
sleep 2
systemctl is-active trusttunnel
if command -v ufw >/dev/null && ufw status | grep -q "Status: active"; then
  echo "==> Открываю порт в ufw"; ufw allow ` + strconv.Itoa(p.Port) + `/tcp; ufw allow ` + strconv.Itoa(p.Port) + `/udp
fi
echo "==> Готово"
`
}
