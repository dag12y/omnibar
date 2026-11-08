// Package config implements TOML configuration loading, defaults,
// and validation for Omnibar.
//
// Configuration lives at $XDG_CONFIG_HOME/omnibar/config.toml
// (~/.config/omnibar/config.toml when XDG_CONFIG_HOME is unset).
//
// Validation is application-defined: TOML has no built-in schema
// feature. Omnibar parses TOML and then validates the parsed
// structure against its own expectations.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	// DefaultConfigVersion is the config file schema version applied
	// when config_version is absent.
	DefaultConfigVersion = 1

	// DefaultLogLevel is the log level applied when log_level is absent.
	DefaultLogLevel = "info"

	// configFileName is the name of the configuration file.
	configFileName = "config.toml"

	// configSubdir is the subdirectory under XDG_CONFIG_HOME.
	configSubdir = "omnibar"
)

// validLogLevels lists the log level names accepted by the loader.
var validLogLevels = map[string]bool{
	"debug":  true,
	"info":   true,
	"warn":   true,
	"warning": true,
	"error":  true,
}

// Config is the top-level Omnibar configuration.
type Config struct {
	Version   int                    `toml:"config_version"`
	Core      CoreConfig             `toml:"core"`
	Modules   map[string]ModuleConfig `toml:"modules"`
	Permissions Permissions             `toml:"permissions"`
}

// CoreConfig contains core application settings.
type CoreConfig struct {
	LogLevel string `toml:"log_level"`
}

// ModuleConfig contains per-module settings.
type ModuleConfig struct {
	Enabled bool `toml:"enabled"`
}

// Permissions maps a quoted dotted permission name to its granted state.
// Keys look like "system.info", "window.manage", etc.
//
// A missing key means the permission is not granted (fail-closed).
type Permissions map[string]bool

// IsGranted reports whether the named permission is granted.
// Missing keys return false (fail-closed semantics).
func (p Permissions) IsGranted(name string) bool {
	return p[name]
}

// DefaultConfig returns a Config with all default values applied.
func DefaultConfig() *Config {
	return &Config{
		Version:   DefaultConfigVersion,
		Core: CoreConfig{
			LogLevel: DefaultLogLevel,
		},
		Modules:     make(map[string]ModuleConfig),
		Permissions: make(Permissions),
	}
}

// Load reads and parses a TOML configuration file from the given path.
//
// If the file does not exist, it returns a config with defaults applied
// (a missing config file is not an error).
//
// If the file exists but is malformed, it returns an error. Malformed
// configuration is never silently ignored.
func Load(path string) (*Config, error) {
	c := DefaultConfig()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return c, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	if err := toml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}

	return c, nil
}

// Validate checks the configuration for invalid values.
//
// It returns an error if any value is invalid. Invalid configuration
// is rejected, not silently corrected.
func (c *Config) Validate() error {
	if c.Version < 0 {
		return fmt.Errorf("config_version must be non-negative, got %d", c.Version)
	}

	if !validLogLevels[c.Core.LogLevel] {
		return fmt.Errorf("invalid log_level %q: must be one of debug, info, warn, error", c.Core.LogLevel)
	}

	for key := range c.Permissions {
		if key == "" {
			return fmt.Errorf("permission key must not be empty")
		}
	}

	return nil
}

// ConfigPath resolves the configuration file path.
//
// It uses $XDG_CONFIG_HOME/omnibar/config.toml, or
// ~/.config/omnibar/config.toml if XDG_CONFIG_HOME is unset.
func ConfigPath() (string, error) {
	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfigHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("determine config directory: %w", err)
		}
		xdgConfigHome = filepath.Join(home, ".config")
	}
	return filepath.Join(xdgConfigHome, configSubdir, configFileName), nil
}
