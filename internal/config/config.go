// Package config defines the rdw server configuration and its defaults.
package config

import "time"

const (
	DefaultPort              = 7681
	DefaultScrollbackCap     = 10_000
	DefaultTokenExpiry       = 24 * time.Hour
	DefaultFilterChainMax    = 8
	DefaultReconnectQueueLen = 1_000
	SchemaVersion            = 1
)

// Config is the top-level server configuration.
type Config struct {
	Server         ServerConfig         `yaml:"server"`
	Auth           AuthConfig           `yaml:"auth"`
	KV             KVConfig             `yaml:"kv"`
	Log            LogConfig            `yaml:"log"`
	Bindings       map[string][]string  `yaml:"bindings,omitempty"`
	UserFormatters []UserFormatterConfig `yaml:"formatters,omitempty"`
}

// UserFormatterConfig defines one user-defined external command formatter.
type UserFormatterConfig struct {
	Name string `yaml:"name"`
	Cmd  string `yaml:"cmd"`
}

// ServerConfig holds daemon networking and operational settings.
type ServerConfig struct {
	Port              int  `yaml:"port"`
	NetworkExpose     bool `yaml:"network_expose"`
	OpenBrowser       bool `yaml:"open_browser"`
	FilterChainMax    int  `yaml:"filter_chain_max"`
	ScrollbackCap     int  `yaml:"scrollback_cap"`
	ReconnectQueueLen int  `yaml:"reconnect_queue_len"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	NoAuth         bool   `yaml:"no_auth"`
	AdminLocalOnly bool   `yaml:"admin_local_only"`
	AdminToken     string `yaml:"admin_token"`
}

// KVConfig holds key-value store settings.
type KVConfig struct {
	PersistPath string `yaml:"persist_path"`
}

// LogConfig holds structured logging settings.
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Default returns a Config populated with safe production defaults.
func Default() Config {
	return Config{
		Server: ServerConfig{
			Port:              DefaultPort,
			NetworkExpose:     false,
			OpenBrowser:       false,
			FilterChainMax:    DefaultFilterChainMax,
			ScrollbackCap:     DefaultScrollbackCap,
			ReconnectQueueLen: DefaultReconnectQueueLen,
		},
		Auth: AuthConfig{
			NoAuth:         false,
			AdminLocalOnly: true,
		},
		KV: KVConfig{
			PersistPath: "",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "console",
		},
		Bindings: nil,
	}
}
