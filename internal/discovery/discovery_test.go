package discovery_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nkh/rdw/internal/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Registry read/write
// ---------------------------------------------------------------------------

func withTempRegistry(t *testing.T) func() {
	t.Helper()
	orig := discovery.RegistryPath()
	_ = orig

	// Override cache dir via env so RegistryPath() points to a temp location.
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	return func() {}
}

func TestRegister_WritesEntry(t *testing.T) {
	withTempRegistry(t)

	info := discovery.ServerInfo{
		Port:      9000,
		PID:       os.Getpid(),
		StartedAt: time.Now().Format(time.RFC3339),
	}

	err := discovery.Register(info)
	require.NoError(t, err)

	servers, err := discovery.ReadRegistry()
	require.NoError(t, err)
	require.Len(t, servers, 1)
	assert.Equal(t, 9000, servers[0].Port)
}

func TestRegister_ReplacesExistingPort(t *testing.T) {
	withTempRegistry(t)

	info1 := discovery.ServerInfo{Port: 9000, PID: 1001, StartedAt: "2024-01-01T00:00:00Z"}
	info2 := discovery.ServerInfo{Port: 9000, PID: 1002, StartedAt: "2024-01-02T00:00:00Z"}

	require.NoError(t, discovery.Register(info1))
	require.NoError(t, discovery.Register(info2))

	servers, _ := discovery.ReadRegistry()
	require.Len(t, servers, 1)
	assert.Equal(t, 1002, servers[0].PID)
}

func TestRegister_MultiplePortsCoexist(t *testing.T) {
	withTempRegistry(t)

	for _, port := range []int{7681, 7682, 7683} {
		require.NoError(t, discovery.Register(discovery.ServerInfo{
			Port: port, PID: os.Getpid(), StartedAt: "2024-01-01T00:00:00Z",
		}))
	}

	servers, _ := discovery.ReadRegistry()
	assert.Len(t, servers, 3)
}

func TestDeregister_RemovesEntry(t *testing.T) {
	withTempRegistry(t)

	require.NoError(t, discovery.Register(discovery.ServerInfo{Port: 9001, PID: 1, StartedAt: "t"}))
	require.NoError(t, discovery.Register(discovery.ServerInfo{Port: 9002, PID: 2, StartedAt: "t"}))

	err := discovery.Deregister(9001)
	require.NoError(t, err)

	servers, _ := discovery.ReadRegistry()
	require.Len(t, servers, 1)
	assert.Equal(t, 9002, servers[0].Port)
}

func TestDeregister_MissingPort_NoError(t *testing.T) {
	withTempRegistry(t)
	assert.NoError(t, discovery.Deregister(9999))
}

func TestReadRegistry_MissingFile_ReturnsEmpty(t *testing.T) {
	withTempRegistry(t)
	servers, err := discovery.ReadRegistry()
	assert.NoError(t, err)
	assert.Empty(t, servers)
}

func TestReadRegistry_MalformedFile_ReturnsEmpty(t *testing.T) {
	withTempRegistry(t)

	path := discovery.RegistryPath()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("not json {{{{"), 0600))

	servers, err := discovery.ReadRegistry()
	assert.NoError(t, err)
	assert.Empty(t, servers)
}

func TestReadRegistry_RoundTrip(t *testing.T) {
	withTempRegistry(t)

	infos := []discovery.ServerInfo{
		{Port: 7681, PID: 100, StartedAt: "2024-01-01T00:00:00Z", SocketPath: "/tmp/rdw/7681.sock"},
		{Port: 7682, PID: 101, StartedAt: "2024-01-02T00:00:00Z", SocketPath: "/tmp/rdw/7682.sock"},
	}

	for _, info := range infos {
		require.NoError(t, discovery.Register(info))
	}

	got, err := discovery.ReadRegistry()
	require.NoError(t, err)
	require.Len(t, got, 2)

	ports := map[int]bool{got[0].Port: true, got[1].Port: true}
	assert.True(t, ports[7681])
	assert.True(t, ports[7682])
}

func TestRegistryPath_IsUnderCacheDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	path := discovery.RegistryPath()
	assert.Contains(t, path, "rdw")
	assert.Contains(t, path, "servers.json")
}

func TestRegistry_ValidJSON(t *testing.T) {
	withTempRegistry(t)

	require.NoError(t, discovery.Register(discovery.ServerInfo{
		Port: 7681, PID: os.Getpid(), StartedAt: time.Now().Format(time.RFC3339),
	}))

	data, err := os.ReadFile(discovery.RegistryPath())
	require.NoError(t, err)

	var raw []json.RawMessage
	assert.NoError(t, json.Unmarshal(data, &raw))
}

// ---------------------------------------------------------------------------
// PruneStale
// ---------------------------------------------------------------------------

func TestPruneStale_RemovesDeadProcesses(t *testing.T) {
	withTempRegistry(t)

	// PID 1 is init/systemd — always alive on Linux but we can use a
	// guaranteed-dead PID: use a very large number unlikely to exist.
	deadPID := 9_999_999

	require.NoError(t, discovery.Register(discovery.ServerInfo{
		Port: 8000, PID: deadPID, StartedAt: "t",
	}))
	require.NoError(t, discovery.Register(discovery.ServerInfo{
		Port: 8001, PID: os.Getpid(), StartedAt: "t",
	}))

	require.NoError(t, discovery.PruneStale())

	servers, _ := discovery.ReadRegistry()
	require.Len(t, servers, 1)
	assert.Equal(t, 8001, servers[0].Port)
}
