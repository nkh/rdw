package server

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// UnixCommand is a command sent over the Unix domain socket by the CLI.
type UnixCommand struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params,omitempty"`
}

// UnixResponse is the server's reply over the Unix socket.
type UnixResponse struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

// startUnixListener opens the owner-privilege Unix domain socket.
// Only the process owner can connect (enforced by file permission 0600).
// Returns the listener and the socket path, or an error.
func startUnixListener(sessionID string) (net.Listener, string, error) {
	dir, err := unixSocketDir()
	if err != nil {
		return nil, "", fmt.Errorf("unix socket dir: %w", err)
	}

	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, "", fmt.Errorf("creating socket dir %q: %w", dir, err)
	}

	path := filepath.Join(dir, sessionID+".sock")

	// Remove stale socket file if it exists.
	_ = os.Remove(path)

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, "", fmt.Errorf("listening on %q: %w", path, err)
	}

	// Restrict to owner only.
	if err = os.Chmod(path, 0600); err != nil {
		ln.Close()
		return nil, "", fmt.Errorf("chmod %q: %w", path, err)
	}

	return ln, path, nil
}

// serveUnix handles a single Unix socket connection from the CLI.
// The connection is short-lived: one command, one response, then close.
func serveUnix(c net.Conn, handler func(UnixCommand) UnixResponse) {
	defer c.Close()

	var cmd UnixCommand
	if err := json.NewDecoder(c).Decode(&cmd); err != nil {
		resp := UnixResponse{OK: false, Error: "invalid command: " + err.Error()}
		_ = json.NewEncoder(c).Encode(resp)
		return
	}

	resp := handler(cmd)
	_ = json.NewEncoder(c).Encode(resp)
}

// acceptUnix runs the accept loop for the Unix listener until it is closed.
func acceptUnix(ln net.Listener, handler func(UnixCommand) UnixResponse) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return // listener closed
		}
		go serveUnix(c, handler)
	}
}

// unixSocketDir returns $XDG_RUNTIME_DIR/rdw, falling back to /tmp/rdw.
func unixSocketDir() (string, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "rdw"), nil
}
