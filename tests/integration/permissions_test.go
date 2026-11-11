package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omnibar/internal/config"
	"omnibar/internal/ipc"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
)

const (
	capabilityModuleName = "testcap"
	testCapability       = "system.info"
)

// capabilityModule is a minimal module that declares a configurable set of
// capabilities. It exists because the shipped core module declares none, so
// there is no production RPC method that can exercise a capability gate yet.
type capabilityModule struct {
	capabilities []string
}

func (m *capabilityModule) Info() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:         capabilityModuleName,
		Description:  "Integration-test module with selectable capabilities.",
		Version:      "0.0.0",
		Capabilities: m.capabilities,
		Enabled:      true,
	}
}

// startCapabilityBackend wires a real registry, dispatcher, and Unix socket
// server using the supplied TOML configuration. The only test-only pieces are
// the module and the test.useCapability method; configuration loading,
// registry enforcement, framing, and JSON-RPC are the production code paths.
func startCapabilityBackend(t *testing.T, configTOML string, capabilities []string) string {
	t.Helper()

	base := t.TempDir()
	configPath := filepath.Join(base, "config.toml")
	if configTOML != "" {
		if err := os.WriteFile(configPath, []byte(configTOML), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate config: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := modules.NewRegistry(cfg, logger)
	if err := registry.Register(&capabilityModule{capabilities: capabilities}); err != nil {
		t.Fatalf("register capability module: %v", err)
	}
	if err := registry.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize capability module: %v", err)
	}
	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("start capability module: %v", err)
	}

	dispatcher := jsonrpc.NewDispatcher()
	ipc.RegisterHandshake(dispatcher, registry)
	ipc.RegisterModuleMethods(dispatcher, registry)

	// Stand-in for a production method that consumes a capability. A real
	// module would make the same registry.HasCapability call at the core
	// boundary; the denial path is identical.
	dispatcher.Register("test.useCapability", func(json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
		if !registry.HasCapability(capabilityModuleName, testCapability) {
			return nil, jsonrpc.NewError(
				jsonrpc.CodeCustomMin,
				"Permission denied: "+testCapability,
				capabilityModuleName,
			)
		}
		return json.RawMessage(`{"granted":true}`), nil
	})

	socketPath := filepath.Join(base, "run", "omnibar.sock")
	server := ipc.New(socketPath, logger, dispatcher.Handle)
	if err := server.Start(); err != nil {
		t.Fatalf("start capability server: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Shutdown(); err != nil {
			t.Errorf("shutdown capability server: %v", err)
		}
	})

	return socketPath
}

func capabilityConfig(permissions string, disableModule bool) string {
	var b strings.Builder
	b.WriteString("config_version = 1\n")
	if disableModule {
		b.WriteString("\n[modules.")
		b.WriteString(capabilityModuleName)
		b.WriteString("]\nenabled = false\n")
	}
	if permissions != "" {
		b.WriteString("\n[permissions]\n")
		b.WriteString(permissions)
		b.WriteString("\n")
	}
	return b.String()
}

// TestFailClosedCapabilities verifies capability enforcement over a real Unix
// socket with real configuration loading and the production registry. Each
// subtest starts a fresh server because configuration is snapshotted at
// startup.
func TestFailClosedCapabilities(t *testing.T) {
	cases := []struct {
		name             string
		config           string
		capabilities     []string
		wantGranted      bool
		wantDeclared     int
		wantGrantedCount int
	}{
		{
			name:             "missing permissions table",
			config:           capabilityConfig("", false),
			capabilities:     []string{testCapability},
			wantGranted:      false,
			wantDeclared:     1,
			wantGrantedCount: 0,
		},
		{
			name:             "explicitly denied",
			config:           capabilityConfig(`"system.info" = false`, false),
			capabilities:     []string{testCapability},
			wantGranted:      false,
			wantDeclared:     1,
			wantGrantedCount: 0,
		},
		{
			name:             "granted declared capability",
			config:           capabilityConfig(`"system.info" = true`, false),
			capabilities:     []string{testCapability},
			wantGranted:      true,
			wantDeclared:     1,
			wantGrantedCount: 1,
		},
		{
			name:             "granted but not declared",
			config:           capabilityConfig(`"system.info" = true`, false),
			capabilities:     nil,
			wantGranted:      false,
			wantDeclared:     0,
			wantGrantedCount: 0,
		},
		{
			// module.getStatus reports the configured grant even though
			// the disabled module cannot actually use the capability.
			name:             "granted but module disabled",
			config:           capabilityConfig(`"system.info" = true`, true),
			capabilities:     []string{testCapability},
			wantGranted:      false,
			wantDeclared:     1,
			wantGrantedCount: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			socketPath := startCapabilityBackend(t, tc.config, tc.capabilities)
			c := dial(t, socketPath)
			handshake(t, c)

			resp, _ := c.request("test.useCapability", nil)
			if tc.wantGranted {
				if resp.Error != nil {
					t.Fatalf("expected capability use to succeed, got error: %+v", resp.Error)
				}
				var result struct {
					Granted bool `json:"granted"`
				}
				if err := json.Unmarshal(resp.Result, &result); err != nil {
					t.Fatalf("unmarshal capability result: %v", err)
				}
				if !result.Granted {
					t.Error("capability result granted = false, want true")
				}
			} else {
				if resp.Error == nil {
					t.Fatal("expected permission denial, got a successful result")
				}
				if resp.Error.Code != jsonrpc.CodeCustomMin {
					t.Errorf("denial code = %d, want %d", resp.Error.Code, jsonrpc.CodeCustomMin)
				}
				if resp.Error.Module != capabilityModuleName {
					t.Errorf("denial module = %q, want %q", resp.Error.Module, capabilityModuleName)
				}
			}

			statusResp, _ := c.request("module.getStatus", map[string]string{"module": capabilityModuleName})
			if statusResp.Error != nil {
				t.Fatalf("module.getStatus failed: %+v", statusResp.Error)
			}
			var status struct {
				DeclaredCapabilities []string `json:"declared_capabilities"`
				GrantedCapabilities  []string `json:"granted_capabilities"`
			}
			if err := json.Unmarshal(statusResp.Result, &status); err != nil {
				t.Fatalf("unmarshal module status: %v", err)
			}
			if len(status.DeclaredCapabilities) != tc.wantDeclared {
				t.Errorf("declared_capabilities = %v, want %d entries", status.DeclaredCapabilities, tc.wantDeclared)
			}
			if len(status.GrantedCapabilities) != tc.wantGrantedCount {
				t.Errorf("granted_capabilities = %v, want %d entries", status.GrantedCapabilities, tc.wantGrantedCount)
			}
		})
	}
}
