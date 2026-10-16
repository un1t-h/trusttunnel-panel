package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHPool держит собственный ключ панели и переиспользует соединения с серверами.
type SSHPool struct {
	signer    ssh.Signer
	PublicKey string // строка для authorized_keys
	mu        sync.Mutex
	conns     map[string]*ssh.Client
	locks     map[string]*sync.Mutex
	onHostKey func(serverID, fp string) // запоминание ключа хоста при первом подключении
}

func NewSSHPool(dir string) (*SSHPool, error) {
	keyPath := filepath.Join(dir, "id_ed25519")
	var signer ssh.Signer
	if b, err := os.ReadFile(keyPath); err == nil {
		signer, err = ssh.ParsePrivateKey(b)
		if err != nil {
			return nil, fmt.Errorf("ключ панели повреждён: %w", err)
		}
	} else {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		block, err := ssh.MarshalPrivateKey(priv, "ttpanel")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600); err != nil {
			return nil, err
		}
		signer, err = ssh.NewSignerFromKey(priv)
		if err != nil {
			return nil, err
		}
	}
	pub := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))) + " ttpanel"
	return &SSHPool{signer: signer, PublicKey: pub, conns: map[string]*ssh.Client{}, locks: map[string]*sync.Mutex{}}, nil
}

func addr(s *Server) string { return net.JoinHostPort(s.Host, strconv.Itoa(s.Port)) }

func (p *SSHPool) config(s *Server, auth ...ssh.AuthMethod) *ssh.ClientConfig {
	expected := s.HostKey
	id := s.ID
	return &ssh.ClientConfig{
		User:    s.User,
		Auth:    auth,
		Timeout: 10 * time.Second,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			fp := ssh.FingerprintSHA256(key)
			if expected == "" {
				if p.onHostKey != nil {
					p.onHostKey(id, fp)
				}
				return nil
			}
			if fp != expected {
				return fmt.Errorf("ключ SSH-сервера изменился (%s, ожидался %s) — возможна подмена; если сервер переустанавливали, удалите и добавьте его заново", fp, expected)
			}
			return nil
		},
	}
}

// ServerLock сериализует операции над одним сервером (запись конфигов, перезапуск).
func (p *SSHPool) ServerLock(id string) *sync.Mutex {
	p.mu.Lock()
	defer p.mu.Unlock()
	l, ok := p.locks[id]
	if !ok {
		l = &sync.Mutex{}
		p.locks[id] = l
	}
	return l
}

func (p *SSHPool) client(s *Server) (*ssh.Client, error) {
	p.mu.Lock()
	c := p.conns[s.ID]
	p.mu.Unlock()
	if c != nil {
		return c, nil
	}
	c, err := ssh.Dial("tcp", addr(s), p.config(s, ssh.PublicKeys(p.signer)))
	if err != nil {
		return nil, err
	}
	// keepalive, чтобы соединение не висело мёртвым
	go func() {
		for {
			time.Sleep(30 * time.Second)
			if _, _, err := c.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				p.Drop(s.ID)
				return
			}
		}
	}()
	p.mu.Lock()
	if old := p.conns[s.ID]; old != nil {
		p.mu.Unlock()
		c.Close()
		return old, nil
	}
	p.conns[s.ID] = c
	p.mu.Unlock()
	return c, nil
}

func (p *SSHPool) Drop(id string) {
	p.mu.Lock()
	c := p.conns[id]
	delete(p.conns, id)
	p.mu.Unlock()
	if c != nil {
		c.Close()
	}
}

// Run выполняет команду; при обрыве кэшированного соединения пробует ещё раз.
func (p *SSHPool) Run(s *Server, cmd, stdin string, timeout time.Duration) (string, string, error) {
	out, errOut, err := p.run(s, cmd, stdin, timeout, nil)
	var exit *ssh.ExitError
	if err != nil && !errors.As(err, &exit) && !errors.Is(err, errTimeout) {
		p.Drop(s.ID)
		out, errOut, err = p.run(s, cmd, stdin, timeout, nil)
	}
	return out, errOut, err
}

// Stream выполняет долгую команду, передавая вывод по мере появления.
func (p *SSHPool) Stream(s *Server, cmd string, timeout time.Duration, w io.Writer) error {
	_, _, err := p.run(s, cmd, "", timeout, w)
	return err
}

var errTimeout = errors.New("превышено время ожидания команды")

func (p *SSHPool) run(s *Server, cmd, stdin string, timeout time.Duration, stream io.Writer) (string, string, error) {
	c, err := p.client(s)
	if err != nil {
		return "", "", err
	}
	sess, err := c.NewSession()
	if err != nil {
		return "", "", err
	}
	defer sess.Close()
	var stdout, stderr bytes.Buffer
	if stream != nil {
		sess.Stdout = stream
		sess.Stderr = stream
	} else {
		sess.Stdout = &stdout
		sess.Stderr = &stderr
	}
	if stdin != "" {
		sess.Stdin = strings.NewReader(stdin)
	}
	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()
	select {
	case err = <-done:
	case <-time.After(timeout):
		sess.Close()
		return stdout.String(), stderr.String(), errTimeout
	}
	return stdout.String(), stderr.String(), err
}

// InstallKey заходит по паролю (один раз) и добавляет ключ панели в authorized_keys.
// Пароль нигде не сохраняется.
func (p *SSHPool) InstallKey(s *Server, password string) error {
	c, err := ssh.Dial("tcp", addr(s), p.config(s, ssh.Password(password),
		ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
			ans := make([]string, len(qs))
			for i := range ans {
				ans[i] = password
			}
			return ans, nil
		})))
	if err != nil {
		return err
	}
	defer c.Close()
	sess, err := c.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	cmd := "umask 077; mkdir -p ~/.ssh && touch ~/.ssh/authorized_keys && (grep -qxF " + shq(p.PublicKey) +
		" ~/.ssh/authorized_keys || echo " + shq(p.PublicKey) + " >> ~/.ssh/authorized_keys)"
	if out, err := sess.CombinedOutput(cmd); err != nil {
		return fmt.Errorf("не удалось добавить ключ: %v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// shq — экранирование строки для sh в одинарных кавычках.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Ping — время установки TCP-соединения с SSH-портом.
func Ping(s *Server) (time.Duration, error) {
	start := time.Now()
	c, err := net.DialTimeout("tcp", addr(s), 4*time.Second)
	if err != nil {
		return 0, err
	}
	c.Close()
	return time.Since(start), nil
}
