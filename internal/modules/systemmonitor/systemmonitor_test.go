package systemmonitor

import (
	"context"
	"testing"

	"omnibar/internal/config"
	"omnibar/internal/events"
	"omnibar/internal/modules"
)

func newTestHub() *events.Hub {
	return events.New(nil)
}

func TestModuleInfo(t *testing.T) {
	m := New(config.ModuleConfig{}, nil, nil)
	info := m.Info()

	if info.Name != Name {
		t.Errorf("Name = %q, want %q", info.Name, Name)
	}
	if info.Version != Version {
		t.Errorf("Version = %q, want %q", info.Version, Version)
	}
	if !info.Enabled {
		t.Error("Expected module to be enabled by default")
	}
	if len(info.Capabilities) != 1 || info.Capabilities[0] != Capability {
		t.Errorf("Capabilities = %v, want [%q]", info.Capabilities, Capability)
	}

	// Must satisfy modules.Module and modules.Lifecycle.
	var _ modules.Module = m
	var _ modules.Lifecycle = m
}

func TestModuleLifecycle(t *testing.T) {
	m := New(config.ModuleConfig{}, nil, nil)

	if err := m.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}
	if err := m.Dispose(context.Background()); err != nil {
		t.Fatalf("Dispose() returned error: %v", err)
	}
}

func TestModuleSnapshotBeforeSampling(t *testing.T) {
	hub := newTestHub()
	m := New(config.ModuleConfig{}, nil, hub)

	// No sampler has run yet, so Snapshot takes one on demand.
	snap := m.Snapshot()
	if snap == nil {
		t.Fatal("Snapshot() returned nil")
	}
	if snap.SampledAt == "" {
		t.Error("SampledAt must not be empty")
	}
	// Memory values should come from the real /proc/meminfo on this
	// machine, which is fine for a smoke test.
	if snap.Memory.Total == 0 {
		t.Error("Memory.Total should be non-zero on a real system")
	}
}
