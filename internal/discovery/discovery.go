// Package discovery locates running rdw server instances.
//
// Multiple rdw servers can run simultaneously on different ports. Every client
// command accepts an optional --port flag; when omitted the client probes the
// default port (7681). If nothing is found there an error is returned rather
// than silently operating against the wrong instance.
package discovery

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const (
	DefaultPort    = 7681
	probeTimeout   = 2 * time.Second
	registryDir    = "rdw"
	registryFile   = "servers.json"
)

// ServerInfo describes a running rdw server instance.
type ServerInfo struct {
	Port      int    `json:"port"`
	PID       int    `json:"pid"`
	StartedAt string `json:"started_at"`
	SocketPath string `json:"socket_path"`
}

// Resolve returns the port to use for client commands.
//
// If port > 0 it is returned directly (explicit --port flag).
// If port == 0 the default port (7681) is probed; if a server answers it is
// returned. If the probe fails all registered servers are listed in the error
// so the caller can suggest the correct --port.
func Resolve(port int) (int, error) {
	if port > 0 {
		if err := probe(port); err != nil {
			return 0, fmt.Errorf("no rdw server responding on port %d: %w", port, err)
		}

		return port, nil
	}

	// Try the default port first.
	if err := probe(DefaultPort); err == nil {
		return DefaultPort, nil
	}

	// Default port is not answering. Check registry for alternatives.
	servers, _ := ReadRegistry()
	if len(servers) == 0 {
		return 0, fmt.Errorf(
			"no rdw server found on default port %d; start one with: rdw server start",
			DefaultPort,
		)
	}

	msg := fmt.Sprintf(
		"no rdw server on default port %d; running servers:\n", DefaultPort,
	)

	for _, s := range servers {
		msg += fmt.Sprintf("  port %d  pid %d  started %s\n", s.Port, s.PID, s.StartedAt)
	}

	msg += "use --port <port> to select one"

	return 0, fmt.Errorf("%s", msg)
}

// probe checks whether a server is responding on the given port.
func probe(port int) error {
	client := &http.Client{Timeout: probeTimeout}
	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/api/v1/ping"
	resp, err := client.Get(url)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return nil
}

// RegistryPath returns the path to the server registry JSON file.
func RegistryPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}

	return filepath.Join(dir, registryDir, registryFile)
}

// ReadRegistry returns all registered server instances. Missing or malformed
// registry files return an empty slice without error.
func ReadRegistry() ([]ServerInfo, error) {
	data, err := os.ReadFile(RegistryPath())
	if err != nil {
		return nil, nil
	}

	var servers []ServerInfo
	if err = json.Unmarshal(data, &servers); err != nil {
		return nil, nil
	}

	return servers, nil
}

// Register writes a server entry into the registry. Called by the server on
// startup.
func Register(info ServerInfo) error {
	servers, _ := ReadRegistry()

	// Replace any existing entry for this port.
	updated := make([]ServerInfo, 0, len(servers)+1)

	for _, s := range servers {
		if s.Port != info.Port {
			updated = append(updated, s)
		}
	}

	updated = append(updated, info)

	return writeRegistry(updated)
}

// Deregister removes the entry for the given port. Called by the server on
// shutdown.
func Deregister(port int) error {
	servers, _ := ReadRegistry()

	updated := make([]ServerInfo, 0, len(servers))

	for _, s := range servers {
		if s.Port != port {
			updated = append(updated, s)
		}
	}

	return writeRegistry(updated)
}

// PruneStale removes registry entries whose processes are no longer running.
func PruneStale() error {
	servers, _ := ReadRegistry()
	if len(servers) == 0 {
		return nil
	}

	alive := make([]ServerInfo, 0, len(servers))

	for _, s := range servers {
		if processAlive(s.PID) {
			alive = append(alive, s)
		}
	}

	return writeRegistry(alive)
}

// processAlive reports whether a process with the given PID is running.
// On Unix, sending signal 0 checks existence without disturbing the process.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = proc.Signal(syscall.Signal(0))

	return err == nil
}

func writeRegistry(servers []ServerInfo) error {
	path := RegistryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating registry directory: %w", err)
	}

	data, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling registry: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}
