package ipc

import (
	"context"
	"encoding/json"
	"testing"

	"omnibar/internal/config"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
)

func newModuleStatusRegistry(t *testing.T) *modules.Registry {
	t.Helper()

	cfg := config.DefaultConfig()
	cfg.Permissions["system.info"] = true

	registry := modules.NewRegistry(cfg, nil)
	if err := registry.Register(handshakeModule{info: modules.ModuleInfo{
		Name:         "alpha",
		Description:  "Alpha module",
		Version:      "1.0.0",
		Capabilities: []string{"system.info", "window.manage"},
		Enabled:      true,
	}}); err != nil {
		t.Fatalf("register alpha: %v", err)
	}
	return registry
}

func TestModuleGetStatus(t *testing.T) {
	registry := newModuleStatusRegistry(t)
	if err := registry.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	d := jsonrpc.NewDispatcher()
	RegisterModuleMethods(d, registry)

	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":7,"method":"module.getStatus","params":{"module":"alpha"}}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	var resp jsonrpc.Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}
	if string(resp.ID) != "7" {
		t.Errorf("response id = %s, want 7", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	var status moduleStatusResult
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		t.Fatalf("result unmarshal: %v", err)
	}
	if status.Name != "alpha" || status.Description != "Alpha module" || status.Version != "1.0.0" {
		t.Errorf("status = %+v", status)
	}
	if !status.Enabled || !status.Running {
		t.Errorf("enabled/running = %v/%v, want true/true", status.Enabled, status.Running)
	}
	if status.State != string(modules.StateRunning) {
		t.Errorf("state = %q, want running", status.State)
	}
	if len(status.DeclaredCapabilities) != 2 {
		t.Errorf("declared capabilities = %v", status.DeclaredCapabilities)
	}
	if len(status.GrantedCapabilities) != 1 || status.GrantedCapabilities[0] != "system.info" {
		t.Errorf("granted capabilities = %v, want [system.info]", status.GrantedCapabilities)
	}
}

func TestModuleGetStatusUnknownModule(t *testing.T) {
	registry := newModuleStatusRegistry(t)

	d := jsonrpc.NewDispatcher()
	RegisterModuleMethods(d, registry)

	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"module.getStatus","params":{"module":"missing"}}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	var resp jsonrpc.Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for unknown module")
	}
	if resp.Error.Code != jsonrpc.CodeInvalidRequest {
		t.Errorf("error code = %d, want %d", resp.Error.Code, jsonrpc.CodeInvalidRequest)
	}
	if resp.Error.Module != "missing" {
		t.Errorf("error module = %q, want missing", resp.Error.Module)
	}
}

func TestModuleGetStatusDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Modules["alpha"] = config.ModuleConfig{Enabled: false}
	cfg.Permissions["system.info"] = true

	registry := modules.NewRegistry(cfg, nil)
	if err := registry.Register(handshakeModule{info: modules.ModuleInfo{
		Name:         "alpha",
		Capabilities: []string{"system.info"},
		Enabled:      true,
	}}); err != nil {
		t.Fatalf("register alpha: %v", err)
	}

	d := jsonrpc.NewDispatcher()
	RegisterModuleMethods(d, registry)

	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"module.getStatus","params":{"module":"alpha"}}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	var resp jsonrpc.Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	var status moduleStatusResult
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		t.Fatalf("result unmarshal: %v", err)
	}
	if status.Enabled {
		t.Error("expected disabled module")
	}
	if status.Running {
		t.Error("disabled module must not be running")
	}
	if status.State != string(modules.StateRegistered) {
		t.Errorf("state = %q, want registered", status.State)
	}
}

func TestModuleGetStatusMissingModuleParam(t *testing.T) {
	registry := newModuleStatusRegistry(t)

	d := jsonrpc.NewDispatcher()
	RegisterModuleMethods(d, registry)

	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"module.getStatus","params":{}}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	var resp jsonrpc.Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for missing module param")
	}
	if resp.Error.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, jsonrpc.CodeInvalidParams)
	}
}
