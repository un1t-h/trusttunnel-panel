package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/pem"
	"flag"
	"io/fs"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

// version задаётся при сборке релиза: -ldflags "-X main.version=1.2.3"
var version = "dev"

type App struct {
	store  *Store
	ssh    *SSHPool
	auth   *Auth
	jobs   *Jobs
	stats  *Stats
	stMu   sync.Mutex
	status map[string]ServerStatus
}

func main() {
	listen := flag.String("listen", ":8443", "адрес веб-интерфейса")
	dataDir := flag.String("data", "/var/lib/ttpanel", "каталог данных")
	plainHTTP := flag.Bool("http", false, "без TLS (если панель стоит за reverse proxy)")
	flag.Parse()

	store, err := OpenStore(*dataDir)
	if err != nil {
		log.Fatalf("данные: %v", err)
	}
	pool, err := NewSSHPool(*dataDir)
	if err != nil {
		log.Fatalf("ssh-ключ: %v", err)
	}
	app := &App{store: store, ssh: pool, auth: NewAuth(), jobs: NewJobs(), stats: NewStats(), status: map[string]ServerStatus{}}
	pool.onHostKey = func(id, fp string) {
		store.Update(func(d *Data) error {
			if s := d.Server(id); s != nil && s.HostKey == "" {
				s.HostKey = fp
			}
			return nil
		})
	}

	go app.backgroundLoop()
	go app.statsLoop()

	static, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	app.routes(mux)
	mux.Handle("/", noCache(http.FileServer(http.FS(static))))

	srv := &http.Server{
		Addr:              *listen,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("TrustTunnel Panel %s слушает %s", version, *listen)
	// при остановке службы сохраняем накопленную статистику
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
		<-sig
		store.FlushIfDirty()
		os.Exit(0)
	}()
	if *plainHTTP {
		log.Fatal(srv.ListenAndServe())
	}
	cert, key, err := ensureCert(*dataDir)
	if err != nil {
		log.Fatalf("сертификат: %v", err)
	}
	srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	log.Fatal(srv.ListenAndServeTLS(cert, key))
}

// backgroundLoop: опрос серверов раз в минуту и отключение клиентов с истёкшим сроком.
func (a *App) backgroundLoop() {
	a.refreshAll()
	statusTick := time.NewTicker(60 * time.Second)
	expiryTick := time.NewTicker(60 * time.Second)
	for {
		select {
		case <-statusTick.C:
			a.refreshAll()
		case <-expiryTick.C:
			a.expireClients()
			a.jobs.Cleanup()
		}
	}
}

func (a *App) refreshAll() {
	var servers []Server
	a.store.View(func(d *Data) {
		for _, s := range d.Servers {
			servers = append(servers, *s)
		}
	})
	var wg sync.WaitGroup
	for i := range servers {
		wg.Add(1)
		go func(s Server) {
			defer wg.Done()
			a.refreshOne(&s)
		}(servers[i])
	}
	wg.Wait()
}

func (a *App) refreshOne(s *Server) ServerStatus {
	st := a.probe(s)
	a.stMu.Lock()
	a.status[s.ID] = st
	a.stMu.Unlock()
	return st
}

func (a *App) expireClients() {
	now := time.Now()
	changed := map[string]bool{}
	a.store.Update(func(d *Data) error {
		for _, c := range d.Clients {
			if c.Enabled && c.ExpiresAt != nil && now.After(*c.ExpiresAt) {
				c.Enabled = false
				changed[c.ServerID] = true
				log.Printf("клиент %s отключён: истёк срок", c.Username)
			}
		}
		return nil
	})
	for id := range changed {
		if err := a.syncServer(id); err != nil {
			log.Printf("синхронизация %s: %v", id, err)
		}
	}
}

// syncServer записывает текущий список клиентов на сервер.
func (a *App) syncServer(serverID string) error {
	var srv Server
	var clients []*Client
	found := false
	a.store.View(func(d *Data) {
		if s := d.Server(serverID); s != nil {
			srv = *s
			found = true
			for _, c := range d.ServerClients(serverID) {
				cc := *c
				clients = append(clients, &cc)
			}
		}
	})
	if !found {
		return nil
	}
	l := a.ssh.ServerLock(serverID)
	l.Lock()
	defer l.Unlock()
	return a.writeCredentials(&srv, clients)
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; frame-ancestors 'none'")
		h.ServeHTTP(w, r)
	})
}

// ensureCert создаёт самоподписанный сертификат для HTTPS (один раз).
func ensureCert(dir string) (string, string, error) {
	certPath, keyPath := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	if _, err := os.Stat(certPath); err == nil {
		return certPath, keyPath, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "TrustTunnel Panel"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}
	if h, err := os.Hostname(); err == nil {
		tpl.DNSNames = append(tpl.DNSNames, h, h+".local")
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				tpl.IPAddresses = append(tpl.IPAddresses, ipn.IP)
			}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0600); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}
