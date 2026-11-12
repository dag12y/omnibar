package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omnibar/internal/ipc/jsonrpc"
)

func TestHandshakeAndCoreStatus(t *testing.T) {
	env := newTestEnv(t)
	startBackend(t, env)

	c := dial(t, env.socketPath)
	result := handshake(t, c)
	if len(result.Modules) != 3 || result.Modules[0] != "core" || result.Modules[1] != "system_monitor" || result.Modules[2] != "keep_awake" {
		t.Fatalf("handshake modules = %v, want [core system_monitor keep_awake]", result.Modules)
	}

	resp, raw := c.request("module.getStatus", map[string]string{"module": "core"})
	if resp.Error != nil {
		t.Fatalf("module.getStatus failed: code=%d message=%q", resp.Error.Code, resp.Error.Message)
	}
	if _, hasMethod := raw["method"]; hasMethod {
		t.Fatalf("response must not contain a method field: %s", rawJSON(raw))
	}

	var status struct {
		Name                 string   `json:"name"`
		Enabled              bool     `json:"enabled"`
		State                string   `json:"state"`
		Running              bool     `json:"running"`
		DeclaredCapabilities []string `json:"declared_capabilities"`
		GrantedCapabilities  []string `json:"granted_capabilities"`
	}
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		t.Fatalf("unmarshal core status: %v", err)
	}
	if status.Name != "core" {
		t.Errorf("status name = %q, want core", status.Name)
	}
	if !status.Enabled {
		t.Error("core should be enabled")
	}
	if !status.Running {
		t.Error("core should be running")
	}
	if status.State != "running" {
		t.Errorf("core state = %q, want running", status.State)
	}
	if status.DeclaredCapabilities == nil || len(status.DeclaredCapabilities) != 0 {
		t.Errorf("core declared_capabilities = %v, want []", status.DeclaredCapabilities)
	}
	if status.GrantedCapabilities == nil || len(status.GrantedCapabilities) != 0 {
		t.Errorf("core granted_capabilities = %v, want []", status.GrantedCapabilities)
	}
}

func TestProtocolErrors(t *testing.T) {
	env := newTestEnv(t)
	startBackend(t, env)

	c := dial(t, env.socketPath)
	handshake(t, c)

	errorCases := []struct {
		name       string
		method     string
		params     any
		wantCode   int
		wantModule string
	}{
		{
			name:       "unknown module",
			method:     "module.getStatus",
			params:     map[string]string{"module": "missing"},
			wantCode:   jsonrpc.CodeInvalidRequest,
			wantModule: "missing",
		},
		{
			name:     "unknown method",
			method:   "module.explode",
			params:   nil,
			wantCode: jsonrpc.CodeMethodNotFound,
		},
		{
			name:     "missing module param",
			method:   "module.getStatus",
			params:   map[string]string{},
			wantCode: jsonrpc.CodeInvalidParams,
		},
		{
			name:     "absent params",
			method:   "module.getStatus",
			params:   nil,
			wantCode: jsonrpc.CodeInvalidParams,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := c.request(tc.method, tc.params)
			if resp.Error == nil {
				t.Fatalf("%s returned a result, want an error", tc.method)
			}
			if resp.Error.Code != tc.wantCode {
				t.Errorf("error code = %d, want %d (message: %s)", resp.Error.Code, tc.wantCode, resp.Error.Message)
			}
			if resp.Error.Module != tc.wantModule {
				t.Errorf("error module = %q, want %q", resp.Error.Module, tc.wantModule)
			}
		})
	}

	t.Run("protocol version mismatch", func(t *testing.T) {
		resp, _ := c.request("client.handshake", map[string]string{"protocol": "omnibar.ipc/v2"})
		if resp.Error == nil {
			t.Fatal("expected an error for a mismatched protocol version")
		}
		if resp.Error.Code != jsonrpc.CodeInvalidRequest {
			t.Errorf("error code = %d, want %d", resp.Error.Code, jsonrpc.CodeInvalidRequest)
		}
		if !strings.Contains(resp.Error.Message, "Protocol version mismatch") {
			t.Errorf("error message = %q, want protocol mismatch detail", resp.Error.Message)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		c.writeRaw(`{"jsonrpc":`)
		resp, raw := c.readFrame()
		if resp.Error == nil {
			t.Fatal("expected a parse error")
		}
		if resp.Error.Code != jsonrpc.CodeParseError {
			t.Errorf("error code = %d, want %d", resp.Error.Code, jsonrpc.CodeParseError)
		}
		if string(raw["id"]) != "null" {
			t.Errorf("parse error id = %s, want null", raw["id"])
		}
	})

	t.Run("wrong jsonrpc version", func(t *testing.T) {
		c.writeRaw(`{"jsonrpc":"1.0","id":77,"method":"module.getStatus","params":{"module":"core"}}`)
		resp, raw := c.readFrame()
		if resp.Error == nil {
			t.Fatal("expected an invalid-request error")
		}
		if resp.Error.Code != jsonrpc.CodeInvalidRequest {
			t.Errorf("error code = %d, want %d", resp.Error.Code, jsonrpc.CodeInvalidRequest)
		}
		if string(raw["id"]) != "77" {
			t.Errorf("response id = %s, want 77", raw["id"])
		}
	})

	// The connection must remain usable after all protocol errors.
	t.Run("connection remains usable", func(t *testing.T) {
		resp, _ := c.request("module.getStatus", map[string]string{"module": "core"})
		if resp.Error != nil {
			t.Fatalf("module.getStatus after errors failed: %+v", resp.Error)
		}
	})
}

func TestNotificationGetsNoResponse(t *testing.T) {
	env := newTestEnv(t)
	startBackend(t, env)

	c := dial(t, env.socketPath)
	handshake(t, c)

	// A request without an id is a JSON-RPC notification. The backend
	// dispatches it but must not write a response.
	c.notify("module.getStatus", map[string]string{"module": "core"})
	c.expectNoResponse(noResponseWindow)

	// The same rule applies to notification methods that do not exist.
	c.notify("module.unknown", nil)
	c.expectNoResponse(noResponseWindow)

	// The connection still works for ordinary requests afterwards.
	resp, _ := c.request("module.getStatus", map[string]string{"module": "core"})
	if resp.Error != nil {
		t.Fatalf("module.getStatus after notifications failed: %+v", resp.Error)
	}
}

func TestDisabledModuleIsReported(t *testing.T) {
	env := newTestEnv(t)
	env.writeConfig(t, `
config_version = 1

[modules.core]
enabled = false
`)

	startBackend(t, env)
	c := dial(t, env.socketPath)

	// The handshake lists every registered module, including disabled ones.
	handshake(t, c)

	resp, _ := c.request("module.getStatus", map[string]string{"module": "core"})
	if resp.Error != nil {
		t.Fatalf("module.getStatus failed: %+v", resp.Error)
	}

	var status struct {
		Enabled bool   `json:"enabled"`
		Running bool   `json:"running"`
		State   string `json:"state"`
	}
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		t.Fatalf("unmarshal core status: %v", err)
	}
	if status.Enabled {
		t.Error("core should be disabled by config")
	}
	if status.Running {
		t.Error("disabled core should not be running")
	}
	if status.State != "registered" {
		t.Errorf("disabled core state = %q, want registered", status.State)
	}
}

func TestSocketSecurity(t *testing.T) {
	env := newTestEnv(t)
	startBackend(t, env)

	socketInfo, err := os.Stat(env.socketPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if got := socketInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("socket permissions = %o, want 0600", got)
	}
	if socketInfo.Mode()&os.ModeSocket == 0 {
		t.Error("socket path is not a Unix socket")
	}

	dirInfo, err := os.Stat(filepath.Dir(env.socketPath))
	if err != nil {
		t.Fatalf("stat runtime directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("runtime directory permissions = %o, want 0700", got)
	}

	lockInfo, err := os.Stat(filepath.Join(filepath.Dir(env.socketPath), "omnibar.lock"))
	if err != nil {
		t.Fatalf("stat lock file: %v", err)
	}
	if got := lockInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("lock file permissions = %o, want 0600", got)
	}
}
