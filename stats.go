package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Статистика клиентов из встроенного Prometheus-эндпоинта TrustTunnel ([metrics], per_client_metrics = true).
// Эндпоинт слушает только 127.0.0.1 на сервере, панель читает его по уже открытому SSH-соединению.

// metricsAddr — shell-фрагмент: адрес эндпоинта метрик из vpn.toml в переменной $MA.
const metricsAddr = `MA=$(awk '/^\[metrics\]/{f=1;next} /^\[/{f=0} f&&/^[[:space:]]*address/{sub(/^[^"]*"/,"");sub(/".*/,"");print;exit}' vpn.toml 2>/dev/null)
[ -z "$MA" ] && MA=127.0.0.1:1987
MA=$(echo "$MA" | sed 's/^0\.0\.0\.0:/127.0.0.1:/; s/^\[::\]:/127.0.0.1:/')
`

// enableMetricsScript включает per_client_metrics (только на 127.0.0.1) и перезапускает службу.
const enableMetricsScript = `cp -p vpn.toml vpn.toml.bak
if grep -q '^\[metrics\]' vpn.toml; then
  if awk '/^\[metrics\]/{f=1;next} /^\[/{f=0} f&&/^[[:space:]]*per_client_metrics/{x=1} END{exit !x}' vpn.toml; then
    sed -i '/^\[metrics\]/,/^\[/ s/^[[:space:]]*per_client_metrics.*/per_client_metrics = true/' vpn.toml
  else
    sed -i '/^\[metrics\]/a per_client_metrics = true' vpn.toml
  fi
else
  printf '\n# Статистика клиентов для TrustTunnel Panel (доступна только локально)\n[metrics]\naddress = "127.0.0.1:1987"\nrequest_timeout_secs = 3\nper_client_metrics = true\n' >> vpn.toml
fi
`

// ClientLive — оперативные данные клиента (в памяти, не сохраняются).
type ClientLive struct {
	Online   bool    `json:"online"`
	Sessions int     `json:"sessions"`
	IP       string  `json:"ip,omitempty"`
	RxRate   float64 `json:"rxRate"` // скачивание клиентом, байт/с
	TxRate   float64 `json:"txRate"` // отдача клиентом, байт/с
	at       time.Time
}

type Stats struct {
	mu     sync.Mutex
	live   map[string]*ClientLive // по ID клиента
	polled map[string]time.Time   // по ID сервера
	busy   map[string]bool
	errs   map[string]string
	lastUI atomic.Int64 // когда интерфейс последний раз запрашивал данные
}

func NewStats() *Stats {
	return &Stats{live: map[string]*ClientLive{}, polled: map[string]time.Time{}, busy: map[string]bool{}, errs: map[string]string{}}
}

func (st *Stats) Touch() { st.lastUI.Store(time.Now().UnixNano()) }

func (st *Stats) uiActive() bool {
	return time.Since(time.Unix(0, st.lastUI.Load())) < 30*time.Second
}

// Snapshot — копия оперативных данных для API.
func (st *Stats) Snapshot() (map[string]ClientLive, map[string]string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	live := make(map[string]ClientLive, len(st.live))
	for id, l := range st.live {
		live[id] = *l
	}
	errs := make(map[string]string, len(st.errs))
	for id, e := range st.errs {
		errs[id] = e
	}
	return live, errs
}

type endpointClient struct {
	Username string  `json:"username"`
	IP       *string `json:"ip"`
	Sessions int     `json:"sessions"`
	Inbound  int64   `json:"inbound"`
	Outbound int64   `json:"outbound"`
}

// statsLoop: пока панель открыта — опрос раз в 3 с (живая скорость), иначе раз в 30 с
// (чтобы не терять трафик между перезапусками службы и обновлять «был в сети»).
func (a *App) statsLoop() {
	tick := time.NewTicker(3 * time.Second)
	flush := time.NewTicker(60 * time.Second)
	for {
		select {
		case <-tick.C:
			active := a.stats.uiActive()
			var servers []Server
			a.store.View(func(d *Data) {
				for _, s := range d.Servers {
					servers = append(servers, *s)
				}
			})
			a.stMu.Lock()
			for _, s := range servers {
				st, ok := a.status[s.ID]
				if !ok || st.Metrics != "on" {
					continue
				}
				a.stats.mu.Lock()
				due := !a.stats.busy[s.ID] && (active || time.Since(a.stats.polled[s.ID]) >= 30*time.Second)
				if due {
					a.stats.busy[s.ID] = true
				}
				a.stats.mu.Unlock()
				if due {
					go func(s Server) {
						a.pollStats(&s)
						a.stats.mu.Lock()
						a.stats.busy[s.ID] = false
						a.stats.polled[s.ID] = time.Now()
						a.stats.mu.Unlock()
					}(s)
				}
			}
			a.stMu.Unlock()
		case <-flush.C:
			if err := a.store.FlushIfDirty(); err != nil {
				log.Printf("сохранение статистики: %v", err)
			}
		}
	}
}

func (a *App) pollStats(s *Server) {
	out, errOut, err := a.ssh.Run(s, ttPrelude(s)+metricsAddr+`curl -s -m 3 "http://$MA/clients"`, "", 10*time.Second)
	var list []endpointClient
	if err == nil {
		err = json.Unmarshal([]byte(strings.TrimSpace(out)), &list)
		if err != nil {
			err = fmt.Errorf("неожиданный ответ эндпоинта метрик")
		}
	}
	a.stats.mu.Lock()
	if err != nil {
		a.stats.errs[s.ID] = "статистика недоступна: " + strings.TrimSpace(err.Error()+" "+errOut)
		a.stats.mu.Unlock()
		return
	}
	delete(a.stats.errs, s.ID)
	a.stats.mu.Unlock()

	byName := make(map[string]endpointClient, len(list))
	for _, e := range list {
		byName[e.Username] = e
	}
	now := time.Now()
	a.store.Mutate(func(d *Data) {
		a.stats.mu.Lock()
		defer a.stats.mu.Unlock()
		for _, c := range d.ServerClients(s.ID) {
			e, ok := byName[c.Username]
			if !ok {
				// клиента нет в списке сервера (например, отключён) — офлайн
				a.stats.live[c.ID] = &ClientLive{at: now}
				continue
			}
			dIn, dOut := e.Inbound-c.LastIn, e.Outbound-c.LastOut
			if dIn < 0 { // счётчики обнулились — служба перезапускалась
				dIn = e.Inbound
			}
			if dOut < 0 {
				dOut = e.Outbound
			}
			c.TrafficIn += dIn
			c.TrafficOut += dOut
			c.LastIn, c.LastOut = e.Inbound, e.Outbound
			if e.Sessions > 0 {
				t := now
				c.LastSeen = &t
			}
			l := &ClientLive{Online: e.Sessions > 0, Sessions: e.Sessions, at: now}
			if e.IP != nil {
				l.IP = *e.IP
			}
			if prev := a.stats.live[c.ID]; prev != nil && !prev.at.IsZero() {
				if dt := now.Sub(prev.at).Seconds(); dt > 0.5 && dt < 120 {
					l.RxRate = float64(dOut) / dt
					l.TxRate = float64(dIn) / dt
				}
			}
			a.stats.live[c.ID] = l
		}
	})
}

// enableMetrics включает статистику и перезапускает службу через 2 с.
// Обнуление счётчиков после перезапуска pollStats распознаёт сам.
func (a *App) enableMetrics(s *Server) error {
	cmd := ttPrelude(s) + enableMetricsScript + `systemd-run --quiet --on-active=2 --timer-property=AccuracySec=100ms systemctl restart trusttunnel 2>/dev/null ||
  (nohup sh -c 'sleep 2; systemctl restart trusttunnel' >/dev/null 2>&1 &)`
	out, errOut, err := a.ssh.Run(s, cmd, "", 30*time.Second)
	if err != nil {
		return fmt.Errorf("не удалось включить статистику: %v %s %s", err, strings.TrimSpace(out), strings.TrimSpace(errOut))
	}
	return nil
}
