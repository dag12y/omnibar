package ipc

import (
	"encoding/json"
	"testing"

	"omnibar/internal/config"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
)

// handshakeModule is a minimal static module used by handshake tests.
type handshakeModule struct {
	info modules.ModuleInfo
}

func (m handshakeModule) Info() modules.ModuleInfo {
	return m.info
}

func newHandshakeRegistry(t *testing.T) *modules.Registry {
	t.Helper()

	registry := modules.NewRegistry(nil, nil)
	if err := registry.Register(handshakeModule{info: modules.ModuleInfo{
		Name:    "alpha",
		Version: "1.0.0",
		Enabled: true,
	}}); err != nil {
		t.Fatalf("register alpha: %v", err)
	}
	if err := registry.Register(handshakeModule{info: modules.ModuleInfo{
		Name:    "beta",
		Version: "2.0.0",
		Enabled: true,
	}}); err != nil {
		t.Fatalf("register beta: %v", err)
	}
	return registry
}

func TestHandshakeSuccess(t *testing.T) {
	d := jsonrpc.NewDispatcher()
	RegisterHandshake(d, newHandshakeRegistry(t))

	params := `{"protocol":"omnibar.ipc/v1","client_id":"omnibar-ui-1"}`
	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"client.handshake","params":` + params + `}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	if out == nil {
		t.Fatal("expected a handshake response")
	}

	var resp jsonrpc.Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}
	if string(resp.ID) != "1" {
		t.Errorf("response id = %v, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %+v", resp.Error)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("result unmarshal: %v", err)
	}
	if string(result["protocol"]) != `"omnibar.ipc/v1"` {
		t.Errorf("result protocol = %s, want omnibar.ipc/v1", result["protocol"])
	}
	if string(result["version"]) != `"`+ServerVersion+`"` {
		t.Errorf("result version = %s, want %q", result["version"], ServerVersion)
	}

	var names []string
	if err := json.Unmarshal(result["modules"], &names); err != nil {
		t.Fatalf("modules unmarshal: %v", err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("modules = %v, want [alpha beta]", names)
	}
}

func TestHandshakeIncludesDisabledModules(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Modules["beta"] = config.ModuleConfig{Enabled: false}

	registry := modules.NewRegistry(cfg, nil)
	if err := registry.Register(handshakeModule{info: modules.ModuleInfo{
		Name:    "alpha",
		Enabled: true,
	}}); err != nil {
		t.Fatalf("register alpha: %v", err)
	}
	if err := registry.Register(handshakeModule{info: modules.ModuleInfo{
		Name:    "beta",
		Enabled: true,
	}}); err != nil {
		t.Fatalf("register beta: %v", err)
	}

	d := jsonrpc.NewDispatcher()
	RegisterHandshake(d, registry)

	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"client.handshake","params":{"protocol":"omnibar.ipc/v1"}}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	var resp jsonrpc.Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}

	var result handshakeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("result unmarshal: %v", err)
	}
	if len(result.Modules) != 2 {
		t.Errorf("modules = %v, want both registered modules", result.Modules)
	}
	if registry.IsEnabled("beta") {
		t.Error("beta should be disabled")
	}
}

func TestHandshakeVersionMismatch(t *testing.T) {
	d := jsonrpc.NewDispatcher()
	RegisterHandshake(d, newHandshakeRegistry(t))

	params := `{"protocol":"omnibar.ipc/v2","client_id":"omnibar-ui-1"}`
	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"client.handshake","params":` + params + `}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	var resp jsonrpc.Response
	json.Unmarshal(out, &resp)
	if resp.Error == nil {
		t.Fatal("expected error response for version mismatch")
	}
	if resp.Error.Code != jsonrpc.CodeInvalidRequest {
		t.Errorf("error code = %d, want %d (-32600)", resp.Error.Code, jsonrpc.CodeInvalidRequest)
	}
	if string(resp.ID) != "1" {
		t.Errorf("response id = %v, want 1", resp.ID)
	}
}

func TestHandshakeMissingParams(t *testing.T) {
	d := jsonrpc.NewDispatcher()
	RegisterHandshake(d, newHandshakeRegistry(t))

	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"client.handshake"}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	var resp jsonrpc.Response
	json.Unmarshal(out, &resp)
	if resp.Error == nil {
		t.Fatal("expected error response for missing params")
	}
	if resp.Error.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("error code = %d, want %d (-32602)", resp.Error.Code, jsonrpc.CodeInvalidParams)
	}
}

func TestHandshakeEmptyRegistryReturnsArray(t *testing.T) {
	d := jsonrpc.NewDispatcher()
	RegisterHandshake(d, modules.NewRegistry(nil, nil))

	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"client.handshake","params":{"protocol":"omnibar.ipc/v1"}}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	var resp jsonrpc.Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}

	var result handshakeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("result unmarshal: %v", err)
	}
	if result.Modules == nil {
		t.Error("modules = null, want []")
	}
	if len(result.Modules) != 0 {
		t.Errorf("modules = %v, want empty", result.Modules)
	}
}
