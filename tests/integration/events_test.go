package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"omnibar/internal/config"
	"omnibar/internal/events"
	"omnibar/internal/ipc"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
)

// readEvent reads one frame and requires a well-formed module.event
// notification (no id, no result) from the server.
func (c *rpcClient) readEvent() (module, event string, data json.RawMessage) {
	c.t.Helper()

	_, raw := c.readFrame()
	if _, hasID := raw["id"]; hasID {
		c.t.Fatalf("notification must not carry an id: %s", rawJSON(raw))
	}
	if _, hasResult := raw["result"]; hasResult {
		c.t.Fatalf("notification must not carry a result: %s", rawJSON(raw))
	}
	var method string
	_ = json.Unmarshal(raw["method"], &method)
	if method != "module.event" {
		c.t.Fatalf("method = %q, want module.event: %s", method, rawJSON(raw))
	}
	var params struct {
		Module string          `json:"module"`
		Event  string          `json:"event"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw["params"], &params); err != nil {
		c.t.Fatalf("unmarshal params: %v", err)
	}
	return params.Module, params.Event, params.Data
}

func subscribeFilter(t *testing.T, resp jsonrpc.Response, wantModule string) []string {
	t.Helper()

	if resp.Error != nil {
		t.Fatalf("unexpected error: code=%d message=%q", resp.Error.Code, resp.Error.Message)
	}
	var result struct {
		Module string   `json:"module"`
		Events []string `json:"events"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal subscription result: %v", err)
	}
	if result.Module != wantModule {
		t.Errorf("subscription module = %q, want %q", result.Module, wantModule)
	}
	return result.Events
}

// TestSubscribeOverRealBinary drives the shipped backend. The shipped core
// module publishes no events, so this checks the protocol, not delivery.
func TestSubscribeOverRealBinary(t *testing.T) {
	env := newTestEnv(t)
	startBackend(t, env)

	c := dial(t, env.socketPath)
	handshake(t, c)

	resp, _ := c.request("module.subscribe", map[string]any{"module": "core"})
	if got := subscribeFilter(t, resp, "core"); !reflect.DeepEqual(got, []string{"*"}) {
		t.Errorf("filter = %v, want [*]", got)
	}

	resp, _ = c.request("module.subscribe", map[string]any{"module": "core", "events": []string{"x"}})
	if got := subscribeFilter(t, resp, "core"); !reflect.DeepEqual(got, []string{"*"}) {
		t.Errorf("filter after named subscribe on a wildcard = %v, want [*]", got)
	}

	resp, _ = c.request("module.unsubscribe", map[string]any{"module": "core"})
	if got := subscribeFilter(t, resp, "core"); len(got) != 0 {
		t.Errorf("remaining filter = %v, want []", got)
	}

	for _, tc := range []struct {
		name     string
		method   string
		params   any
		wantCode int
	}{
		{"unknown module", "module.subscribe", map[string]string{"module": "missing"}, jsonrpc.CodeInvalidRequest},
		{"missing module", "module.subscribe", map[string]string{}, jsonrpc.CodeInvalidParams},
		{"empty events", "module.subscribe", map[string]any{"module": "core", "events": []string{}}, jsonrpc.CodeInvalidParams},
		{"unknown module on unsubscribe", "module.unsubscribe", map[string]string{"module": "missing"}, jsonrpc.CodeInvalidRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := c.request(tc.method, tc.params)
			if resp.Error == nil || resp.Error.Code != tc.wantCode {
				t.Errorf("error = %+v, want code %d", resp.Error, tc.wantCode)
			}
		})
	}
}

func TestSubscribeToDisabledModuleOverRealBinary(t *testing.T) {
	env := newTestEnv(t)
	env.writeConfig(t, "config_version = 1\n\n[modules.core]\nenabled = false\n")
	startBackend(t, env)

	c := dial(t, env.socketPath)
	handshake(t, c)

	resp, _ := c.request("module.subscribe", map[string]string{"module": "core"})
	if resp.Error == nil || resp.Error.Code != jsonrpc.CodeModuleUnavailable {
		t.Fatalf("error = %+v, want code %d", resp.Error, jsonrpc.CodeModuleUnavailable)
	}
	if resp.Error.Module != "core" {
		t.Errorf("error module = %q, want core", resp.Error.Module)
	}
}

// TestShutdownWithActiveSubscriptionExitsCleanly checks the real process:
// SIGTERM with a subscribed client exits 0, removes the socket, and logs no
// errors.
func TestShutdownWithActiveSubscriptionExitsCleanly(t *testing.T) {
	env := newTestEnv(t)
	p := startBackend(t, env)

	c := dial(t, env.socketPath)
	handshake(t, c)
	c.request("module.subscribe", map[string]string{"module": "core"})

	p.stopGracefully(t)

	c.expectClosed(2 * time.Second)
	if log := p.output(); strings.Contains(log, "level=ERROR") {
		t.Errorf("shutdown with a subscription logged errors:\n%s", log)
	}
}

// TestSubscriptionDoesNotSurviveRestart models what the UI relies on:
// subscriptions live in the backend's memory, so after a restart the client
// must handshake and subscribe again, and that works.
func TestSubscriptionDoesNotSurviveRestart(t *testing.T) {
	env := newTestEnv(t)

	p1 := startBackend(t, env)
	c1 := dial(t, env.socketPath)
	handshake(t, c1)
	c1.request("module.subscribe", map[string]string{"module": "core"})
	p1.stopGracefully(t)
	c1.expectClosed(2 * time.Second)

	p2 := startBackend(t, env)
	c2 := dialWithRetry(t, env.socketPath, 5*time.Second)
	handshake(t, c2)
	resp, _ := c2.request("module.subscribe", map[string]string{"module": "core"})
	if got := subscribeFilter(t, resp, "core"); !reflect.DeepEqual(got, []string{"*"}) {
		t.Errorf("re-subscribe filter = %v, want [*]", got)
	}
	p2.stopGracefully(t)
}

// --- in-process backend with a real event publisher -----------------------

const (
	publisherModuleName = "pubmod"
	publisherCapability = "system.info"
)

// publisherModule declares a capability and exposes a Publisher once the
// registry has started it, the way a real producer module will.
type publisherModule struct {
	caps []string
}

func (m *publisherModule) Info() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:         publisherModuleName,
		Description:  "Integration-test event producer.",
		Version:      "0.0.0",
		Capabilities: m.caps,
		Enabled:      true,
	}
}

type eventBackend struct {
	socketPath string
	hub        *events.Hub
	registry   *modules.Registry
	publisher  *events.Publisher
}

// startEventBackend wires the production registry, event hub, dispatcher,
// and socket server around publisherModule.
func startEventBackend(t *testing.T, configTOML string, caps []string) *eventBackend {
	t.Helper()

	base := t.TempDir()
	configPath := filepath.Join(base, "config.toml")
	if configTOML != "" {
		writeFile(t, configPath, configTOML)
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
	if err := registry.Register(&publisherModule{caps: caps}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := registry.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	hub := events.New(logger)
	publisher := hub.Publisher(publisherModuleName, func() bool {
		state, _ := registry.State(publisherModuleName)
		return state == modules.StateRunning
	})

	dispatcher := jsonrpc.NewDispatcher()
	ipc.RegisterHandshake(dispatcher, registry)
	ipc.RegisterModuleMethods(dispatcher, registry)
	ipc.RegisterEventMethods(dispatcher, registry, hub)

	socketPath := filepath.Join(base, "run", "omnibar.sock")
	server := ipc.NewWithContext(socketPath, logger, dispatcher.HandleContext)
	if err := server.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Shutdown(); err != nil {
			t.Errorf("shutdown server: %v", err)
		}
	})

	return &eventBackend{socketPath: socketPath, hub: hub, registry: registry, publisher: publisher}
}

func TestEventDeliveryOverRealSocket(t *testing.T) {
	b := startEventBackend(t, "", nil)

	subscribed := dial(t, b.socketPath)
	handshake(t, subscribed)
	resp, _ := subscribed.request("module.subscribe", map[string]any{
		"module": publisherModuleName,
		"events": []string{"reading"},
	})
	if got := subscribeFilter(t, resp, publisherModuleName); !reflect.DeepEqual(got, []string{"reading"}) {
		t.Fatalf("filter = %v, want [reading]", got)
	}

	bystander := dial(t, b.socketPath)
	handshake(t, bystander)

	if err := b.publisher.Publish("ignored", 0); err != nil {
		t.Fatalf("Publish ignored: %v", err)
	}
	for i := 1; i <= 3; i++ {
		if err := b.publisher.Publish("reading", map[string]int{"value": i}); err != nil {
			t.Fatalf("Publish reading: %v", err)
		}
	}

	for i := 1; i <= 3; i++ {
		module, event, data := subscribed.readEvent()
		if module != publisherModuleName || event != "reading" {
			t.Fatalf("event %d = %s/%s", i, module, event)
		}
		var d struct{ Value int }
		if err := json.Unmarshal(data, &d); err != nil || d.Value != i {
			t.Fatalf("event %d data = %s, want value %d in publish order", i, data, i)
		}
	}
	subscribed.expectNoResponse(noResponseWindow)
	bystander.expectNoResponse(noResponseWindow)

	// Replies and events share one ordered stream: a request issued after
	// publishing still gets its own reply.
	resp, _ = subscribed.request("module.getStatus", map[string]string{"module": publisherModuleName})
	if resp.Error != nil {
		t.Fatalf("getStatus on a subscribed connection failed: %+v", resp.Error)
	}
}

func TestEventsStopAfterUnsubscribeAndDisconnect(t *testing.T) {
	b := startEventBackend(t, "", nil)

	c := dial(t, b.socketPath)
	handshake(t, c)
	c.request("module.subscribe", map[string]string{"module": publisherModuleName})
	if !b.hub.HasSubscribers(publisherModuleName) {
		t.Fatal("subscription not registered")
	}

	c.request("module.unsubscribe", map[string]string{"module": publisherModuleName})
	_ = b.publisher.Publish("reading", 1)
	c.expectNoResponse(noResponseWindow)

	c.request("module.subscribe", map[string]string{"module": publisherModuleName})
	c.conn.Close()
	deadline := time.Now().Add(3 * time.Second)
	for b.hub.HasSubscribers(publisherModuleName) {
		if time.Now().After(deadline) {
			t.Fatal("subscription not cleaned up after disconnect")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSubscribeRequiresGrantedCapabilities(t *testing.T) {
	cases := []struct {
		name     string
		config   string
		wantCode int // 0 means success
	}{
		{"no permissions table", "config_version = 1\n", jsonrpc.CodePermissionDenied},
		{"explicitly denied", "config_version = 1\n\n[permissions]\n\"system.info\" = false\n", jsonrpc.CodePermissionDenied},
		{"granted", "config_version = 1\n\n[permissions]\n\"system.info\" = true\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := startEventBackend(t, tc.config, []string{publisherCapability})
			c := dial(t, b.socketPath)
			handshake(t, c)

			resp, _ := c.request("module.subscribe", map[string]string{"module": publisherModuleName})
			if tc.wantCode == 0 {
				if resp.Error != nil {
					t.Fatalf("subscribe failed: %+v", resp.Error)
				}
				return
			}
			if resp.Error == nil || resp.Error.Code != tc.wantCode {
				t.Fatalf("error = %+v, want code %d", resp.Error, tc.wantCode)
			}
			if resp.Error.Module != publisherModuleName {
				t.Errorf("error module = %q, want %q", resp.Error.Module, publisherModuleName)
			}
			if b.hub.HasSubscribers(publisherModuleName) {
				t.Error("a denied subscription must not be registered")
			}
		})
	}
}

func TestStoppedModuleDoesNotPublish(t *testing.T) {
	b := startEventBackend(t, "", nil)

	c := dial(t, b.socketPath)
	handshake(t, c)
	c.request("module.subscribe", map[string]string{"module": publisherModuleName})

	if err := b.registry.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := b.publisher.Publish("reading", 1); err == nil {
		t.Fatal("publishing from a stopped module should be refused")
	}
	c.expectNoResponse(noResponseWindow)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
