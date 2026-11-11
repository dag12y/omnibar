package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"omnibar/internal/config"
	"omnibar/internal/events"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
)

const testIOTimeout = 3 * time.Second

// eventRig is a real IPC server wired to a registry, hub, and dispatcher
// with the production subscribe handlers.
type eventRig struct {
	t        *testing.T
	path     string
	server   *Server
	hub      *events.Hub
	registry *modules.Registry
	logs     *syncBuffer
}

// newEventRig starts a server with modules:
//
//	alpha    enabled, no capabilities
//	granted  enabled, needs system.info (granted)
//	gated    enabled, needs system.info (granted) and window.manage (not)
//	off      disabled by default
func newEventRig(t *testing.T, queueSize int) *eventRig {
	t.Helper()

	cfg := config.DefaultConfig()
	cfg.Permissions["system.info"] = true

	registry := modules.NewRegistry(cfg, nil)
	for _, info := range []modules.ModuleInfo{
		{Name: "alpha", Enabled: true},
		{Name: "granted", Capabilities: []string{"system.info"}, Enabled: true},
		{Name: "gated", Capabilities: []string{"system.info", "window.manage"}, Enabled: true},
		{Name: "off", Enabled: false},
	} {
		if err := registry.Register(handshakeModule{info: info}); err != nil {
			t.Fatalf("register %s: %v", info.Name, err)
		}
	}
	if err := registry.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	logs := &syncBuffer{}
	logger := newBufferLogger(logs)
	hub := events.New(logger)

	d := jsonrpc.NewDispatcher()
	RegisterHandshake(d, registry)
	RegisterModuleMethods(d, registry)
	RegisterEventMethods(d, registry, hub)

	path, restore := tempSocketPath(t)
	t.Cleanup(restore)

	server := NewWithContext(path, logger, d.HandleContext)
	if queueSize > 0 {
		server.SetQueueSize(queueSize)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })

	return &eventRig{t: t, path: path, server: server, hub: hub, registry: registry, logs: logs}
}

func (r *eventRig) dial() *testClient {
	r.t.Helper()
	conn, err := net.DialTimeout("unix", r.path, testIOTimeout)
	if err != nil {
		r.t.Fatalf("dial: %v", err)
	}
	c := &testClient{t: r.t, conn: conn, reader: bufio.NewReaderSize(conn, 64*1024)}
	r.t.Cleanup(func() { conn.Close() })
	return c
}

type testClient struct {
	t      *testing.T
	conn   net.Conn
	reader *bufio.Reader
	nextID int
}

func (c *testClient) send(frame string) {
	c.t.Helper()
	_ = c.conn.SetWriteDeadline(time.Now().Add(testIOTimeout))
	if _, err := c.conn.Write([]byte(frame + "\n")); err != nil {
		c.t.Fatalf("write: %v", err)
	}
}

// readMessage reads one frame as a generic JSON object.
func (c *testClient) readMessage() map[string]json.RawMessage {
	c.t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(testIOTimeout))
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		c.t.Fatalf("read: %v", err)
	}
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		c.t.Fatalf("frame is not a JSON object: %v\n%q", err, line)
	}
	return msg
}

// call sends a request and returns its response, failing if any
// notification arrives first (callers sequence their expectations).
func (c *testClient) call(method, params string) jsonrpc.Response {
	c.t.Helper()
	c.nextID++
	frame := `{"jsonrpc":"2.0","id":` + strconv.Itoa(c.nextID) + `,"method":"` + method + `"`
	if params != "" {
		frame += `,"params":` + params
	}
	c.send(frame + "}")

	raw := c.readMessage()
	if _, isNotification := raw["method"]; isNotification {
		c.t.Fatalf("expected a response, got notification: %v", raw)
	}
	data, _ := json.Marshal(raw)
	var resp jsonrpc.Response
	if err := json.Unmarshal(data, &resp); err != nil {
		c.t.Fatalf("unmarshal response: %v", err)
	}
	if string(resp.ID) != strconv.Itoa(c.nextID) {
		c.t.Fatalf("response id = %s, want %d", resp.ID, c.nextID)
	}
	return resp
}

type eventNote struct {
	Module string
	Event  string
	Data   json.RawMessage
}

// readEvent reads one frame and requires it to be a module.event
// notification: no id, correct method, and params module/event/data.
func (c *testClient) readEvent() eventNote {
	c.t.Helper()
	raw := c.readMessage()
	if _, hasID := raw["id"]; hasID {
		c.t.Fatalf("notification must not have an id: %v", raw)
	}
	var method string
	_ = json.Unmarshal(raw["method"], &method)
	if method != "module.event" {
		c.t.Fatalf("method = %q, want module.event", method)
	}
	var params struct {
		Module string          `json:"module"`
		Event  string          `json:"event"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw["params"], &params); err != nil {
		c.t.Fatalf("unmarshal params: %v", err)
	}
	return eventNote{Module: params.Module, Event: params.Event, Data: params.Data}
}

func (c *testClient) expectSilence(window time.Duration) {
	c.t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(window))
	if _, err := c.reader.ReadByte(); err == nil {
		c.t.Fatal("expected no data from the server")
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		c.t.Fatalf("expected a timeout, got %v", err)
	}
	_ = c.conn.SetReadDeadline(time.Time{})
}

func resultFilter(t *testing.T, resp jsonrpc.Response, wantModule string) []string {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var r subscriptionResult
	if err := json.Unmarshal(resp.Result, &r); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if r.Module != wantModule {
		t.Errorf("result module = %q, want %q", r.Module, wantModule)
	}
	return r.Events
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(testIOTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestSubscribeDeliversEventsInOrder(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()

	filter := resultFilter(t, c.call("module.subscribe", `{"module":"alpha"}`), "alpha")
	if !reflect.DeepEqual(filter, []string{"*"}) {
		t.Fatalf("filter = %v, want [*]", filter)
	}

	for i := 1; i <= 5; i++ {
		if err := rig.hub.Publish("alpha", "tick", map[string]int{"n": i}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	for i := 1; i <= 5; i++ {
		ev := c.readEvent()
		if ev.Module != "alpha" || ev.Event != "tick" {
			t.Fatalf("event %d = %+v", i, ev)
		}
		if got, want := string(ev.Data), `{"n":`+strconv.Itoa(i)+`}`; got != want {
			t.Errorf("event %d data = %s, want %s (order)", i, got, want)
		}
	}
}

func TestSubscribeEventFilter(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()

	filter := resultFilter(t, c.call("module.subscribe", `{"module":"alpha","events":["b","a"]}`), "alpha")
	if !reflect.DeepEqual(filter, []string{"a", "b"}) {
		t.Fatalf("filter = %v, want [a b]", filter)
	}

	_ = rig.hub.Publish("alpha", "other", 1)
	_ = rig.hub.Publish("alpha", "a", 2)
	if ev := c.readEvent(); ev.Event != "a" {
		t.Fatalf("first delivered event = %q, want a (other must be filtered out)", ev.Event)
	}
	c.expectSilence(100 * time.Millisecond)
}

func TestSubscribeIsAdditiveAndIdempotent(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()

	c.call("module.subscribe", `{"module":"alpha","events":["a"]}`)
	again := resultFilter(t, c.call("module.subscribe", `{"module":"alpha","events":["a"]}`), "alpha")
	if !reflect.DeepEqual(again, []string{"a"}) {
		t.Errorf("repeat subscribe filter = %v, want [a]", again)
	}
	more := resultFilter(t, c.call("module.subscribe", `{"module":"alpha","events":["b"]}`), "alpha")
	if !reflect.DeepEqual(more, []string{"a", "b"}) {
		t.Errorf("additive subscribe filter = %v, want [a b]", more)
	}

	_ = rig.hub.Publish("alpha", "a", nil)
	c.readEvent()
	c.expectSilence(100 * time.Millisecond) // exactly one copy
}

func TestSubscribeGrantedCapabilityIsAllowed(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()
	resultFilter(t, c.call("module.subscribe", `{"module":"granted"}`), "granted")
}

func TestSubscribeErrors(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()

	cases := []struct {
		name       string
		params     string
		wantCode   int
		wantModule string
		wantText   string
	}{
		{"unknown module", `{"module":"nope"}`, jsonrpc.CodeInvalidRequest, "nope", "not found"},
		{"disabled module", `{"module":"off"}`, jsonrpc.CodeModuleUnavailable, "off", "not available"},
		{"permission not granted", `{"module":"gated"}`, jsonrpc.CodePermissionDenied, "gated", "window.manage"},
		{"missing module", `{}`, jsonrpc.CodeInvalidParams, "", "module"},
		{"module wrong type", `{"module":5}`, jsonrpc.CodeInvalidParams, "", ""},
		{"params not an object", `"alpha"`, jsonrpc.CodeInvalidParams, "", ""},
		{"empty events list", `{"module":"alpha","events":[]}`, jsonrpc.CodeInvalidParams, "alpha", "empty"},
		{"empty event name", `{"module":"alpha","events":[""]}`, jsonrpc.CodeInvalidParams, "alpha", "event names"},
		{"reserved wildcard name", `{"module":"alpha","events":["*"]}`, jsonrpc.CodeInvalidParams, "alpha", "event names"},
		{"events wrong type", `{"module":"alpha","events":"a"}`, jsonrpc.CodeInvalidParams, "", ""},
		{"absent params", ``, jsonrpc.CodeInvalidParams, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := c.call("module.subscribe", tc.params)
			if resp.Error == nil {
				t.Fatalf("expected an error, got result %s", resp.Result)
			}
			if resp.Error.Code != tc.wantCode {
				t.Errorf("code = %d, want %d (%s)", resp.Error.Code, tc.wantCode, resp.Error.Message)
			}
			if resp.Error.Module != tc.wantModule {
				t.Errorf("error module = %q, want %q", resp.Error.Module, tc.wantModule)
			}
			if !strings.Contains(resp.Error.Message, tc.wantText) {
				t.Errorf("message = %q, want it to contain %q", resp.Error.Message, tc.wantText)
			}
		})
	}

	// A failed subscribe must leave no subscription behind.
	for _, module := range []string{"nope", "off", "gated"} {
		if rig.hub.HasSubscribers(module) {
			t.Errorf("failed subscribe left a subscriber on %q", module)
		}
	}
}

func TestSubscribeFailsClosedWhenModuleStops(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()

	if err := rig.registry.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	resp := c.call("module.subscribe", `{"module":"alpha"}`)
	if resp.Error == nil || resp.Error.Code != jsonrpc.CodeModuleUnavailable {
		t.Fatalf("subscribe to stopped module = %+v, want code %d", resp.Error, jsonrpc.CodeModuleUnavailable)
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()
	c.call("module.subscribe", `{"module":"alpha"}`)

	filter := resultFilter(t, c.call("module.unsubscribe", `{"module":"alpha"}`), "alpha")
	if filter == nil || len(filter) != 0 {
		t.Fatalf("remaining filter = %#v, want empty non-nil list", filter)
	}

	_ = rig.hub.Publish("alpha", "tick", nil)
	c.expectSilence(100 * time.Millisecond)
	if rig.hub.HasSubscribers("alpha") {
		t.Error("module still has subscribers after unsubscribe")
	}
}

func TestUnsubscribeNamedEvents(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()
	c.call("module.subscribe", `{"module":"alpha","events":["a","b"]}`)

	filter := resultFilter(t, c.call("module.unsubscribe", `{"module":"alpha","events":["a"]}`), "alpha")
	if !reflect.DeepEqual(filter, []string{"b"}) {
		t.Fatalf("remaining = %v, want [b]", filter)
	}
	_ = rig.hub.Publish("alpha", "a", nil)
	_ = rig.hub.Publish("alpha", "b", nil)
	if ev := c.readEvent(); ev.Event != "b" {
		t.Errorf("event = %q, want b", ev.Event)
	}
}

func TestUnsubscribeIsIdempotentAndAlwaysAllowed(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()

	// Not subscribed: succeeds.
	resultFilter(t, c.call("module.unsubscribe", `{"module":"alpha"}`), "alpha")
	// Needs no grant and no running module: a client can always leave.
	resultFilter(t, c.call("module.unsubscribe", `{"module":"gated"}`), "gated")
	resultFilter(t, c.call("module.unsubscribe", `{"module":"off"}`), "off")

	resp := c.call("module.unsubscribe", `{"module":"nope"}`)
	if resp.Error == nil || resp.Error.Code != jsonrpc.CodeInvalidRequest {
		t.Errorf("unsubscribe unknown module = %+v, want code %d", resp.Error, jsonrpc.CodeInvalidRequest)
	}
	resp = c.call("module.unsubscribe", `{}`)
	if resp.Error == nil || resp.Error.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("unsubscribe without module = %+v, want code %d", resp.Error, jsonrpc.CodeInvalidParams)
	}
}

func TestOnlySubscribedClientsReceiveEvents(t *testing.T) {
	rig := newEventRig(t, 0)
	subscribed := rig.dial()
	bystander := rig.dial()
	other := rig.dial()

	subscribed.call("module.subscribe", `{"module":"alpha"}`)
	other.call("module.subscribe", `{"module":"granted"}`)

	_ = rig.hub.Publish("alpha", "tick", nil)

	subscribed.readEvent()
	bystander.expectSilence(100 * time.Millisecond)
	other.expectSilence(100 * time.Millisecond)
}

func TestSubscriptionsAreNotSharedAcrossConnections(t *testing.T) {
	rig := newEventRig(t, 0)
	a := rig.dial()
	b := rig.dial()
	a.call("module.subscribe", `{"module":"alpha"}`)
	b.call("module.subscribe", `{"module":"alpha"}`)

	// A leaving does not affect B.
	a.call("module.unsubscribe", `{"module":"alpha"}`)
	_ = rig.hub.Publish("alpha", "tick", nil)
	b.readEvent()
	a.expectSilence(100 * time.Millisecond)
}

func TestDisconnectRemovesSubscriptions(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()
	c.call("module.subscribe", `{"module":"alpha"}`)
	c.call("module.subscribe", `{"module":"granted","events":["x"]}`)
	if rig.hub.SubscriberCount("alpha") != 1 || rig.hub.SubscriberCount("granted") != 1 {
		t.Fatal("subscriptions were not registered")
	}

	c.conn.Close()

	waitFor(t, "subscriptions to be removed after disconnect", func() bool {
		return !rig.hub.HasSubscribers("alpha") && !rig.hub.HasSubscribers("granted")
	})
}

func TestSubscribeWithoutSessionIsInternalError(t *testing.T) {
	rig := newEventRig(t, 0)

	d := jsonrpc.NewDispatcher()
	RegisterEventMethods(d, rig.registry, rig.hub)

	// Handle (no context) models a caller that is not an IPC connection.
	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"module.subscribe","params":{"module":"alpha"}}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	var resp jsonrpc.Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != jsonrpc.CodeInternalError {
		t.Fatalf("response = %+v, want internal error", resp)
	}
	if rig.hub.HasSubscribers("alpha") {
		t.Error("a subscriber was registered without a session")
	}
}

func TestSubscribeNotificationIsIgnoredSafely(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()

	// A subscribe sent as a notification (no id) gets no response, and
	// must not crash the connection.
	c.send(`{"jsonrpc":"2.0","method":"module.subscribe","params":{"module":"alpha"}}`)
	c.expectSilence(100 * time.Millisecond)

	resp := c.call("module.unsubscribe", `{"module":"alpha"}`)
	if resp.Error != nil {
		t.Fatalf("connection unusable after notification: %+v", resp.Error)
	}
}
