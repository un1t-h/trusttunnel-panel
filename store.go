package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Settings — учётная запись администратора панели.
type Settings struct {
	AdminUser string `json:"adminUser"`
	PassHash  string `json:"passHash"`
}

// Server — VPN-сервер TrustTunnel, которым панель управляет по SSH.
type Server struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	User          string    `json:"user"`
	PublicAddress string    `json:"publicAddress"` // адрес для клиентов: ip/домен[:порт]
	HostKey       string    `json:"hostKey"`       // отпечаток SSH-ключа сервера (запоминается при первом подключении)
	InstallDir    string    `json:"installDir"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Client — пользователь VPN на конкретном сервере.
type Client struct {
	ID        string     `json:"id"`
	ServerID  string     `json:"serverId"`
	Username  string     `json:"username"`
	Password  string     `json:"password"`
	Enabled   bool       `json:"enabled"`
	Note      string     `json:"note"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`

	// Статистика. TrustTunnel обнуляет счётчики при каждом перезапуске службы,
	// поэтому панель сама накапливает суммарный трафик по приращениям.
	TrafficIn  int64      `json:"trafficIn"`  // отправлено клиентом (upload), байт
	TrafficOut int64      `json:"trafficOut"` // получено клиентом (download), байт
	LastIn     int64      `json:"lastIn"`     // последние увиденные значения счётчиков сервера
	LastOut    int64      `json:"lastOut"`
	LastSeen   *time.Time `json:"lastSeen,omitempty"`
}

type Data struct {
	Settings Settings  `json:"settings"`
	Servers  []*Server `json:"servers"`
	Clients  []*Client `json:"clients"`
}

// Store — состояние в JSON-файле (без СУБД: на Pi 3 это быстрее и проще).
type Store struct {
	mu    sync.Mutex
	path  string
	d     Data
	dirty bool
}

// Mutate меняет данные без немедленной записи на диск (для частой статистики —
// бережём SD-карту Raspberry Pi). Сохранение делает FlushIfDirty раз в минуту.
func (s *Store) Mutate(fn func(d *Data)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.d)
	s.dirty = true
}

func (s *Store) FlushIfDirty() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return nil
	}
	return s.saveLocked()
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "data.json")}
	b, err := os.ReadFile(s.path)
	if err == nil {
		if err := json.Unmarshal(b, &s.d); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s, nil
}

// Update выполняет изменение под блокировкой и сохраняет файл.
func (s *Store) Update(fn func(d *Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.d); err != nil {
		return err
	}
	return s.saveLocked()
}

// View даёт доступ на чтение под блокировкой.
func (s *Store) View(fn func(d *Data)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.d)
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(&s.d, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

func (s *Store) Raw() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.MarshalIndent(&s.d, "", "  ")
	return b
}

func (d *Data) Server(id string) *Server {
	for _, s := range d.Servers {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (d *Data) Client(id string) *Client {
	for _, c := range d.Clients {
		if c.ID == id {
			return c
		}
	}
	return nil
}

func (d *Data) ServerClients(serverID string) []*Client {
	var r []*Client
	for _, c := range d.Clients {
		if c.ServerID == serverID {
			r = append(r, c)
		}
	}
	return r
}

func newID() string { return randHex(8) }

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
