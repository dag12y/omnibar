package core

import (
	"testing"

	"omnibar/internal/modules"
)

func TestInfo(t *testing.T) {
	info := New().Info()

	if info.Name != Name {
		t.Errorf("Name = %q, want %q", info.Name, Name)
	}
	if info.Version != Version {
		t.Errorf("Version = %q, want %q", info.Version, Version)
	}
	if info.Description == "" {
		t.Error("Description must not be empty")
	}
	if !info.Enabled {
		t.Error("core module must be enabled by default")
	}
	if len(info.Capabilities) != 0 {
		t.Errorf("Capabilities = %v, want none", info.Capabilities)
	}
}

func TestRegistersInRegistry(t *testing.T) {
	registry := modules.NewRegistry(nil, nil)
	if err := registry.Register(New()); err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	names := registry.Names()
	if len(names) != 1 || names[0] != Name {
		t.Errorf("Names() = %v, want [%s]", names, Name)
	}
	if !registry.IsEnabled(Name) {
		t.Error("core module should be enabled")
	}
}
