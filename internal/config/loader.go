package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads configuration from path (if non-empty) and merges it over the
// defaults. If path is empty, defaults are returned unchanged.
func Load(path string) (Config, error) {
	cfg := Default()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config file %q: %w", path, err)
	}

	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	if err = validate(cfg); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// DefaultConfigPath returns the platform-appropriate default config file path.
// Returns an empty string if the path cannot be determined.
func DefaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}

	return filepath.Join(dir, "rdw", "config.yaml")
}

// validate checks that required config fields are within bounds.
func validate(c Config) error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port %d is out of range [1, 65535]", c.Server.Port)
	}

	if c.Server.FilterChainMax < 1 {
		return fmt.Errorf("server.filter_chain_max must be >= 1")
	}

	if c.Server.FilterChainMax > 64 {
		return fmt.Errorf("server.filter_chain_max must be <= 64")
	}

	if c.Server.ScrollbackCap < 1 {
		return fmt.Errorf("server.scrollback_cap must be >= 1")
	}

	if c.Server.ReconnectQueueLen < 1 {
		return fmt.Errorf("server.reconnect_queue_len must be >= 1")
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Log.Level] {
		return fmt.Errorf("log.level %q is invalid; must be one of: debug, info, warn, error", c.Log.Level)
	}

	validFormats := map[string]bool{"json": true, "console": true}
	if !validFormats[c.Log.Format] {
		return fmt.Errorf("log.format %q is invalid; must be one of: json, console", c.Log.Format)
	}

	return nil
}
