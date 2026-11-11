package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.toml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() missing file returned error: %v", err)
	}

	if cfg.Version != DefaultConfigVersion {
		t.Errorf("Version = %d, want %d", cfg.Version, DefaultConfigVersion)
	}
	if cfg.Core.LogLevel != DefaultLogLevel {
		t.Errorf("Core.LogLevel = %q, want %q", cfg.Core.LogLevel, DefaultLogLevel)
	}
	if len(cfg.Modules) != 0 {
		t.Errorf("Modules = %v, want empty", cfg.Modules)
	}
	if len(cfg.Permissions) != 0 {
		t.Errorf("Permissions = %v, want empty", cfg.Permissions)
	}
	if cfg.Permissions.IsGranted("system.info") {
		t.Error("expected system.info to be denied when permissions table is missing")
	}
}

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
config_version = 1
[core]
  log_level = "debug"
[modules]
  [modules.system_monitor]
  enabled = true
[permissions]
  "system.info" = true
  "window.manage" = false
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Version)
	}
	if cfg.Core.LogLevel != "debug" {
		t.Errorf("Core.LogLevel = %q, want %q", cfg.Core.LogLevel, "debug")
	}
	mod, ok := cfg.Modules["system_monitor"]
	if !ok {
		t.Fatal("expected module system_monitor, not found")
	}
	if !mod.Enabled {
		t.Error("expected system_monitor enabled = true")
	}
	if !cfg.Permissions.IsGranted("system.info") {
		t.Error("expected system.info to be granted")
	}
	if cfg.Permissions.IsGranted("window.manage") {
		t.Error("expected window.manage to be denied")
	}
}

func TestLoadMalformedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("= invalid toml ="), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed config")
	}
	if !strings.Contains(err.Error(), "parse config file") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestValidateInvalidLogLevel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Core.LogLevel = "not-a-level"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
	if !strings.Contains(err.Error(), "invalid log_level") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestValidateNegativeConfigVersion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = -1

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative config_version")
	}
	if !strings.Contains(err.Error(), "config_version") {
		t.Errorf("expected config_version error, got: %v", err)
	}
}

func TestValidateVersionOneAccepted(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = 1

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() returned error for version 1: %v", err)
	}
}

func TestValidateVersionZeroRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = 0

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for config_version 0")
	}
	if !strings.Contains(err.Error(), "unsupported config_version") {
		t.Errorf("expected unsupported version error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "only version 1 is supported") {
		t.Errorf("expected supported version in error, got: %v", err)
	}
}

func TestValidateVersionTwoRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = 2

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for config_version 2")
	}
	if !strings.Contains(err.Error(), "unsupported config_version") {
		t.Errorf("expected unsupported version error, got: %v", err)
	}
}

func TestLoadOmittedVersionDefaultsToOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[core]
log_level = "info"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Version != DefaultConfigVersion {
		t.Errorf("Version = %d, want %d", cfg.Version, DefaultConfigVersion)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Version != DefaultConfigVersion {
		t.Errorf("Version = %d, want %d", cfg.Version, DefaultConfigVersion)
	}
	if cfg.Core.LogLevel != DefaultLogLevel {
		t.Errorf("Core.LogLevel = %q, want %q", cfg.Core.LogLevel, DefaultLogLevel)
	}
	if len(cfg.Modules) != 0 {
		t.Errorf("Modules = %v, want empty", cfg.Modules)
	}
	if len(cfg.Permissions) != 0 {
		t.Errorf("Permissions = %v, want empty", cfg.Permissions)
	}
}

func TestConfigPath(t *testing.T) {
	// Unset XDG_CONFIG_HOME → use ~/.config/omnibar/config.toml
	old := os.Getenv("XDG_CONFIG_HOME")
	os.Unsetenv("XDG_CONFIG_HOME")
	defer func() { os.Setenv("XDG_CONFIG_HOME", old) }()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("get home dir: %v", err)
	}
	expected := filepath.Join(home, ".config", "omnibar", "config.toml")

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() returned error: %v", err)
	}
	if path != expected {
		t.Errorf("ConfigPath() = %q, want %q", path, expected)
	}
}

func TestConfigPathWithXDG(t *testing.T) {
	old := os.Getenv("XDG_CONFIG_HOME")
	defer func() { os.Setenv("XDG_CONFIG_HOME", old) }()

	os.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg")
	expected := filepath.Join("/tmp/test-xdg", "omnibar", "config.toml")

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() returned error: %v", err)
	}
	if path != expected {
		t.Errorf("ConfigPath() = %q, want %q", path, expected)
	}
}

func TestValidateIntervalMsTooLow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Modules["system_monitor"] = ModuleConfig{Enabled: true, IntervalMs: 100}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for interval_ms below minimum")
	}
	if !contains(err.Error(), "interval_ms") {
		t.Errorf("expected interval_ms error, got: %v", err)
	}
}

func TestValidateIntervalMsTooHigh(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Modules["system_monitor"] = ModuleConfig{Enabled: true, IntervalMs: 99999}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for interval_ms above maximum")
	}
	if !contains(err.Error(), "interval_ms") {
		t.Errorf("expected interval_ms error, got: %v", err)
	}
}

func TestValidateIntervalMsBoundariesAccepted(t *testing.T) {
	for _, ms := range []int{0, 500, 2000, 60000} {
		cfg := DefaultConfig()
		cfg.Modules["system_monitor"] = ModuleConfig{Enabled: true, IntervalMs: ms}
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() returned error for interval_ms=%d: %v", ms, err)
		}
	}
}

func TestLoadIntervalMs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
config_version = 1
[modules]
  [modules.system_monitor]
  enabled = true
  interval_ms = 1000
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	mod, ok := cfg.Modules["system_monitor"]
	if !ok {
		t.Fatal("expected module system_monitor, not found")
	}
	if mod.IntervalMs != 1000 {
		t.Errorf("IntervalMs = %d, want 1000", mod.IntervalMs)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestPermissionsFailClosed(t *testing.T) {
	// Missing permissions table → empty map → all denied.
	p := Permissions{}
	if p.IsGranted("system.info") {
		t.Error("expected missing permission to be denied")
	}
	if p.IsGranted("window.manage") {
		t.Error("expected missing permission to be denied")
	}

	// Explicitly denied → denied.
	p2 := Permissions{"window.manage": false}
	if p2.IsGranted("window.manage") {
		t.Error("expected explicitly denied permission to be denied")
	}

	// Explicitly granted → granted.
	p3 := Permissions{"system.info": true}
	if !p3.IsGranted("system.info") {
		t.Error("expected granted permission to be granted")
	}
}
