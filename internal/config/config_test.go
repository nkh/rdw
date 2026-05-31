package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nkh/rdw/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault_Values(t *testing.T) {
	c := config.Default()

	assert.Equal(t, 7681, c.Server.Port)
	assert.False(t, c.Server.NetworkExpose)
	assert.False(t, c.Auth.NoAuth)
	assert.True(t, c.Auth.AdminLocalOnly)
	assert.Equal(t, "", c.KV.PersistPath)
	assert.Equal(t, "info", c.Log.Level)
	assert.Equal(t, "console", c.Log.Format)
	assert.Equal(t, config.DefaultFilterChainMax, c.Server.FilterChainMax)
	assert.Equal(t, config.DefaultScrollbackCap, c.Server.ScrollbackCap)
	assert.Equal(t, config.DefaultReconnectQueueLen, c.Server.ReconnectQueueLen)
}

func TestLoad_NoFile_ReturnsDefaults(t *testing.T) {
	cfg, err := config.Load("")
	require.NoError(t, err)
	assert.Equal(t, config.Default(), cfg)
}

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
server:
  port: 9090
  network_expose: true
log:
  level: debug
  format: json
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0600))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	assert.Equal(t, 9090, cfg.Server.Port)
	assert.True(t, cfg.Server.NetworkExpose)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)

	// Unspecified values must still have defaults
	assert.True(t, cfg.Auth.AdminLocalOnly)
}

func TestLoad_InvalidPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	require.NoError(t, os.WriteFile(path, []byte("server:\n  port: 0\n"), 0600))

	_, err := config.Load(path)
	assert.Error(t, err)
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	require.NoError(t, os.WriteFile(path, []byte("log:\n  level: verbose\n"), 0600))

	_, err := config.Load(path)
	assert.Error(t, err)
}

func TestLoad_InvalidLogFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	require.NoError(t, os.WriteFile(path, []byte("log:\n  format: text\n"), 0600))

	_, err := config.Load(path)
	assert.Error(t, err)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/path/config.yaml")
	assert.Error(t, err)
}

func TestLoad_KVPersistPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	dbPath := filepath.Join(dir, "kv.db")

	yaml := "kv:\n  persist_path: " + dbPath + "\n"
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0600))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, dbPath, cfg.KV.PersistPath)
}

func TestLoad_FilterChainMaxBounds(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		yaml  string
		valid bool
	}{
		{"server:\n  filter_chain_max: 0\n", false},
	}

	for _, tc := range cases {
		path := filepath.Join(dir, "cfg.yaml")
		require.NoError(t, os.WriteFile(path, []byte(tc.yaml), 0600))

		_, err := config.Load(path)

		if tc.valid {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
		}
	}
}
