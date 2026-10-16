package main

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "ttp_session"
const sessionTTL = 7 * 24 * time.Hour

type session struct {
	user    string
	expires time.Time
}

// Auth — сессии в памяти и защита от перебора пароля.
type Auth struct {
	mu       sync.Mutex
	sessions map[string]session
	fails    map[string][]time.Time
}

func NewAuth() *Auth {
	return &Auth{sessions: map[string]session{}, fails: map[string][]time.Time{}}
}

func hashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), 11)
	return string(b), err
}

func checkPassword(hash, p string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Blocked: не более 5 неудачных попыток за 10 минут с одного IP.
func (a *Auth) Blocked(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	cut := time.Now().Add(-10 * time.Minute)
	var keep []time.Time
	for _, t := range a.fails[ip] {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	a.fails[ip] = keep
	return len(keep) >= 5
}

func (a *Auth) Fail(ip string) {
	a.mu.Lock()
	a.fails[ip] = append(a.fails[ip], time.Now())
	a.mu.Unlock()
}

func (a *Auth) Start(w http.ResponseWriter, r *http.Request, user string) {
	tok := randHex(32)
	a.mu.Lock()
	a.sessions[tok] = session{user: user, expires: time.Now().Add(sessionTTL)}
	delete(a.fails, clientIP(r))
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL.Seconds()),
	})
}

func (a *Auth) End(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
}

// EndAll — выход на всех устройствах (после смены пароля).
func (a *Auth) EndAll() {
	a.mu.Lock()
	a.sessions = map[string]session{}
	a.mu.Unlock()
}

func (a *Auth) User(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[c.Value]
	if !ok || time.Now().After(s.expires) {
		delete(a.sessions, c.Value)
		return ""
	}
	return s.user
}
