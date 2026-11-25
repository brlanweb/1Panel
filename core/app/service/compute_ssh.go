package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/ssh"
)

// Pooled SSH connections for agentless (L0) compute hosts.
//
// One TCP connection per host is kept and multiplexed (every command is a
// new session on the same connection), so a 15-second sample costs one
// session open instead of a full TCP + key exchange. Idle connections are
// closed after computeSSHIdle; a broken connection is dropped and redialled
// on the next call. A per-host mutex serialises dialling only — sessions
// themselves run concurrently.

const (
	computeSSHIdle        = 10 * time.Minute
	computeSSHDialTimeout = 8 * time.Second
	computeSSHMaxOutput   = 4 << 20
)

type computeSSHConn struct {
	mu       sync.Mutex
	client   *ssh.SSHClient
	sig      string
	lastUsed time.Time
}

type computeSSHPool struct {
	mu    sync.Mutex
	conns map[uint]*computeSSHConn
}

var computeSSH = &computeSSHPool{conns: map[uint]*computeSSHConn{}}

func (p *computeSSHPool) reapLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		p.mu.Lock()
		for id, c := range p.conns {
			c.mu.Lock()
			if c.client != nil && time.Since(c.lastUsed) > computeSSHIdle {
				c.client.Close()
				c.client = nil
			}
			c.mu.Unlock()
			if c.client == nil {
				delete(p.conns, id)
			}
		}
		p.mu.Unlock()
	}
}

// Drop closes the pooled connection of a host (after edit / delete).
func (p *computeSSHPool) Drop(hostID uint) {
	p.mu.Lock()
	c := p.conns[hostID]
	delete(p.conns, hostID)
	p.mu.Unlock()
	if c != nil {
		c.mu.Lock()
		if c.client != nil {
			c.client.Close()
			c.client = nil
		}
		c.mu.Unlock()
	}
}

func computeConnInfo(host model.ComputeHost) (ssh.ConnInfo, error) {
	info := ssh.ConnInfo{
		User:        host.SSHUser,
		Addr:        host.Addr,
		Port:        host.SSHPort,
		AuthMode:    host.AuthMode,
		DialTimeOut: computeSSHDialTimeout,
	}
	if info.Port == 0 {
		info.Port = 22
	}
	if info.User == "" {
		info.User = "root"
	}
	cred := ""
	if host.CredCipher != "" {
		plain, err := encrypt.StringDecrypt(host.CredCipher)
		if err != nil {
			return info, fmt.Errorf("decrypt credential: %w", err)
		}
		cred = plain
	}
	switch host.AuthMode {
	case "key":
		info.PrivateKey = []byte(cred)
		if host.PassPhraseCipher != "" {
			pass, err := encrypt.StringDecrypt(host.PassPhraseCipher)
			if err != nil {
				return info, fmt.Errorf("decrypt passphrase: %w", err)
			}
			info.PassPhrase = []byte(pass)
		}
	default:
		info.AuthMode = "password"
		info.Password = cred
	}
	return info, nil
}

func computeConnSig(host model.ComputeHost) string {
	return fmt.Sprintf("%s|%d|%s|%s|%s|%s", host.Addr, host.SSHPort, host.SSHUser, host.AuthMode, host.CredCipher, host.PassPhraseCipher)
}

func (p *computeSSHPool) conn(host model.ComputeHost) (*computeSSHConn, *ssh.SSHClient, error) {
	p.mu.Lock()
	c := p.conns[host.ID]
	if c == nil {
		c = &computeSSHConn{}
		p.conns[host.ID] = c
	}
	p.mu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	sig := computeConnSig(host)
	if c.client != nil && c.sig != sig {
		c.client.Close()
		c.client = nil
	}
	if c.client == nil {
		info, err := computeConnInfo(host)
		if err != nil {
			return c, nil, err
		}
		client, err := ssh.NewClient(info)
		if err != nil {
			return c, nil, err
		}
		c.client = client
		c.sig = sig
	}
	c.lastUsed = time.Now()
	return c, c.client, nil
}

func (c *computeSSHConn) invalidate(client *ssh.SSHClient) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == client && client != nil {
		client.Close()
		c.client = nil
	}
}

type computeLimitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (b *computeLimitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

// Run executes a script through `sh -s` (fed on stdin, so quoting and the
// script size are not a problem). When sudo is true and the login user is
// not root, the script runs under `sudo -n`. One retry on a stale
// connection.
func (p *computeSSHPool) Run(ctx context.Context, host model.ComputeHost, script string, sudo bool) (string, error) {
	out, err, retry := p.runOnce(ctx, host, script, sudo)
	if retry && ctx.Err() == nil {
		out, err, _ = p.runOnce(ctx, host, script, sudo)
	}
	return out, err
}

func (p *computeSSHPool) runOnce(ctx context.Context, host model.ComputeHost, script string, sudo bool) (string, error, bool) {
	c, client, err := p.conn(host)
	if err != nil {
		return "", err, false
	}
	session, err := client.Client.NewSession()
	if err != nil {
		// The pooled connection is dead: drop it and let the caller retry once.
		c.invalidate(client)
		return "", err, true
	}
	defer session.Close()
	out := &computeLimitedBuffer{max: computeSSHMaxOutput}
	session.Stdout = out
	session.Stderr = out
	session.Stdin = strings.NewReader(script)
	cmd := "sh -s"
	if sudo && host.SSHUser != "root" && client.SudoItem != "" {
		cmd = "sudo -n sh -s"
	}
	done := make(chan error, 1)
	go func() { done <- session.Run(cmd) }()
	select {
	case err = <-done:
		return out.buf.String(), err, false
	case <-ctx.Done():
		_ = session.Signal("KILL")
		_ = session.Close()
		return out.buf.String(), errors.New("ssh command timeout"), false
	}
}

// RunTimeout is Run with its own deadline.
func (p *computeSSHPool) RunTimeout(host model.ComputeHost, script string, sudo bool, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := p.Run(ctx, host, script, sudo)
	if err != nil {
		global.LOG.Debugf("compute ssh %s: %v", host.Name, err)
	}
	return out, err
}
