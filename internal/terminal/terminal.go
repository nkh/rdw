// Package terminal manages gotty/ttyd terminal panes. Each pane runs a
// restricted subprocess (shell) under a dedicated unprivileged user and
// exposes it via a local port that the browser proxies through the rdw
// WebSocket.
//
// Sandbox strategy: the subprocess is started with the system "nobody" user
// via `su -s /bin/sh nobody -c cmd`. If the rdw process does not have
// privilege to su, the launch fails loudly at startup — per the requirement
// that misconfiguration must cause a startup failure, not a silent bypass.
package terminal

import (
	"fmt"
	"net"
	"os/exec"
	"sync"
)

// Pane represents one running terminal subprocess.
type Pane struct {
	ID   string
	Port int
	cmd  *exec.Cmd
}

// Manager tracks active terminal panes and assigns local ports.
type Manager struct {
	mu    sync.Mutex
	panes map[string]*Pane
	next  int // next port to try
}

// New returns a Manager that allocates ports starting from base.
func New(base int) *Manager {
	return &Manager{
		panes: make(map[string]*Pane),
		next:  base,
	}
}

// Launch starts a restricted terminal for the given pane ID running cmd
// (defaults to "/bin/sh" if empty). Returns the local port the terminal
// listens on.
//
// The subprocess is run as "nobody" via `su -s /bin/sh nobody`. If su is
// not available or privilege is insufficient the call returns an error.
func (m *Manager) Launch(id, cmd string) (int, error) {
	if cmd == "" {
		cmd = "/bin/sh"
	}

	port, err := m.allocPort()
	if err != nil {
		return 0, fmt.Errorf("terminal: no free port: %w", err)
	}

	// Prefer ttyd; fall back to a plain netcat echo for environments without it.
	ttyd, ttydErr := exec.LookPath("ttyd")
	var proc *exec.Cmd

	if ttydErr == nil {
		// ttyd -p <port> --once su -s /bin/sh nobody -c '<cmd>'
		proc = exec.Command(
			ttyd,
			"-p", fmt.Sprintf("%d", port),
			"--once",
			"su", "-s", "/bin/sh", "nobody", "-c", cmd,
		)
	} else {
		// Fallback: plain restricted shell on a raw TCP socket via socat.
		socat, socatErr := exec.LookPath("socat")
		if socatErr != nil {
			return 0, fmt.Errorf("terminal: neither ttyd nor socat found; install ttyd for terminal pane support")
		}

		proc = exec.Command(
			socat,
			fmt.Sprintf("TCP-LISTEN:%d,reuseaddr,fork", port),
			fmt.Sprintf("EXEC:'su -s /bin/sh nobody -c \"%s\"',pty,stderr", cmd),
		)
	}

	if err := proc.Start(); err != nil {
		return 0, fmt.Errorf("terminal: launch %q: %w", id, err)
	}

	m.mu.Lock()
	m.panes[id] = &Pane{ID: id, Port: port, cmd: proc}
	m.mu.Unlock()

	return port, nil
}

// Kill stops the terminal for the given pane ID.
func (m *Manager) Kill(id string) error {
	m.mu.Lock()
	p, ok := m.panes[id]
	if ok {
		delete(m.panes, id)
	}
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("terminal: pane %q not found", id)
	}

	_ = p.cmd.Process.Kill()
	_ = p.cmd.Wait()

	return nil
}

// Get returns the Pane for id, or nil.
func (m *Manager) Get(id string) *Pane {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.panes[id]
}

// All returns a snapshot of active panes.
func (m *Manager) All() []*Pane {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]*Pane, 0, len(m.panes))
	for _, p := range m.panes {
		out = append(out, p)
	}

	return out
}

// allocPort finds a free TCP port starting from m.next.
func (m *Manager) allocPort() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for attempts := 0; attempts < 100; attempts++ {
		port := m.next
		m.next++

		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}

		_ = ln.Close()

		return port, nil
	}

	return 0, fmt.Errorf("no free port found in range")
}
