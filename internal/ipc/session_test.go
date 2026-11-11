package ipc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"omnibar/internal/ipc/jsonrpc"
)

// syncBuffer is a bytes.Buffer safe for concurrent writes by slog and
// reads by tests.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newBufferLogger(b *syncBuffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(b, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// bigPayload returns a JSON string value of about n bytes.
func bigPayload(n int) string {
	return strings.Repeat("x", n)
}

func TestSlowClientIsDisconnectedWithoutBlockingPublisher(t *testing.T) {
	rig := newEventRig(t, 8)

	slow := rig.dial() // subscribes, then never reads
	slow.call("module.subscribe", `{"module":"alpha"}`)

	fast := rig.dial()
	fast.call("module.subscribe", `{"module":"alpha"}`)

	const nEvents = 40
	var received atomic.Int32
	fastDone := make(chan struct{})
	go func() {
		defer close(fastDone)
		for i := 0; i < nEvents; i++ {
			raw := fast.readMessage()
			if string(raw["method"]) == `"module.event"` {
				received.Add(1)
			}
		}
	}()

	payload := bigPayload(64 * 1024)
	start := time.Now()
	for i := 0; i < nEvents; i++ {
		if err := rig.hub.Publish("alpha", "big", payload); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		// Pace the publisher so the healthy client keeps up; only the
		// client that never reads can fall behind.
		time.Sleep(2 * time.Millisecond)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("publishing took %s: a stuck client is blocking the publisher", elapsed)
	}

	select {
	case <-fastDone:
	case <-time.After(testIOTimeout):
		t.Fatalf("healthy client received %d/%d events", received.Load(), nEvents)
	}

	waitFor(t, "slow client to be disconnected", func() bool {
		return rig.hub.SubscriberCount("alpha") == 1
	})
	if !strings.Contains(rig.logs.String(), "client too slow") {
		t.Errorf("expected a slow-client warning in the log:\n%s", rig.logs.String())
	}

	// The server closed the slow client's connection: draining it ends in
	// an error or EOF rather than hanging open.
	_ = slow.conn.SetReadDeadline(time.Now().Add(testIOTimeout))
	buf := make([]byte, 64*1024)
	for {
		if _, err := slow.conn.Read(buf); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				t.Fatal("slow client's connection was left open")
			}
			break
		}
	}
}

func TestRepliesAndEventsNeverInterleave(t *testing.T) {
	rig := newEventRig(t, 4096)
	c := rig.dial()
	c.call("module.subscribe", `{"module":"alpha"}`)

	const (
		nEvents   = 500
		nRequests = 200
	)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		payload := bigPayload(2048)
		for i := 0; i < nEvents; i++ {
			_ = rig.hub.Publish("alpha", "tick", payload)
		}
	}()

	// Fire the requests without waiting for replies, so replies and events
	// compete for the same connection.
	go func() {
		for i := 0; i < nRequests; i++ {
			c.send(`{"jsonrpc":"2.0","id":` + strconv.Itoa(1000+i) + `,"method":"module.getStatus","params":{"module":"alpha"}}`)
		}
	}()

	var events, replies int
	for events+replies < nEvents+nRequests {
		raw := c.readMessage() // fails the test on any frame that is not one intact JSON object
		if _, isNotification := raw["method"]; isNotification {
			events++
		} else {
			replies++
		}
	}
	wg.Wait()

	if events != nEvents || replies != nRequests {
		t.Errorf("got %d events and %d replies, want %d and %d", events, replies, nEvents, nRequests)
	}
}

func TestQueuedReplyIsFlushedAfterClientHalfClose(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()

	c.send(`{"jsonrpc":"2.0","id":1,"method":"module.getStatus","params":{"module":"alpha"}}`)
	if err := c.conn.(*net.UnixConn).CloseWrite(); err != nil {
		t.Fatalf("CloseWrite: %v", err)
	}

	raw := c.readMessage()
	if string(raw["id"]) != "1" || raw["result"] == nil {
		t.Fatalf("reply lost after the client half-closed: %v", raw)
	}
}

func TestOversizedEventIsDroppedWithoutDisconnecting(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()
	c.call("module.subscribe", `{"module":"alpha"}`)

	if err := rig.hub.Publish("alpha", "huge", bigPayload(MaxFrameSize+1024)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	c.expectSilence(150 * time.Millisecond)

	// The client is still connected and receives later events.
	_ = rig.hub.Publish("alpha", "small", 1)
	if ev := c.readEvent(); ev.Event != "small" {
		t.Errorf("event = %q, want small", ev.Event)
	}
	if !strings.Contains(rig.logs.String(), "larger than the frame limit") {
		t.Errorf("expected the oversize drop to be logged:\n%s", rig.logs.String())
	}
}

func TestSessionOnCloseHooks(t *testing.T) {
	logs := &syncBuffer{}
	logger := newBufferLogger(logs)

	var order []string
	var mu sync.Mutex
	record := func(name string) func() {
		return func() {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
		}
	}

	path, restore := tempSocketPath(t)
	defer restore()

	server := NewWithContext(path, logger, func(ctx context.Context, msg []byte) ([]byte, error) {
		sess, ok := SessionFromContext(ctx)
		if !ok {
			t.Error("handler context has no session")
			return nil, nil
		}
		sess.OnClose("first", record("first"))
		sess.OnClose("first", record("first-again")) // duplicate key: ignored
		sess.OnClose("panics", func() { panic("hook failure") })
		sess.OnClose("last", record("last"))
		return []byte(`{}`), nil
	})
	if err := server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer server.Shutdown()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := conn.Write([]byte("hello\nagain\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 64)
	_ = conn.SetReadDeadline(time.Now().Add(testIOTimeout))
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	conn.Close()

	waitFor(t, "close hooks to run", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order) == 2
	})

	mu.Lock()
	defer mu.Unlock()
	if order[0] != "first" || order[1] != "last" {
		t.Errorf("hook order = %v, want [first last] (each once, a panicking hook must not stop the rest)", order)
	}
	if !strings.Contains(logs.String(), "session close hook panicked") {
		t.Errorf("expected the hook panic to be logged:\n%s", logs.String())
	}
}

func TestShutdownWithActiveSubscriptionsIsClean(t *testing.T) {
	rig := newEventRig(t, 0)
	a := rig.dial()
	b := rig.dial()
	a.call("module.subscribe", `{"module":"alpha"}`)
	b.call("module.subscribe", `{"module":"granted"}`)

	// Keep events flowing while the server shuts down.
	stop := make(chan struct{})
	var publishing sync.WaitGroup
	publishing.Add(1)
	go func() {
		defer publishing.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = rig.hub.Publish("alpha", "tick", 1)
				time.Sleep(time.Millisecond)
			}
		}
	}()

	time.Sleep(20 * time.Millisecond)
	done := make(chan error, 1)
	go func() { done <- rig.server.Shutdown() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown hung with active subscriptions")
	}
	close(stop)
	publishing.Wait()

	waitFor(t, "subscriptions to be cleared", func() bool {
		return !rig.hub.HasSubscribers("alpha") && !rig.hub.HasSubscribers("granted")
	})
	if strings.Contains(rig.logs.String(), "level=ERROR") {
		t.Errorf("graceful shutdown logged errors:\n%s", rig.logs.String())
	}
}

func TestDeliverAfterSessionClosedIsSafe(t *testing.T) {
	rig := newEventRig(t, 0)
	c := rig.dial()
	c.call("module.subscribe", `{"module":"alpha"}`)

	// Hammer Publish while the client disconnects: delivery must never
	// panic (send on closed channel) or race with session teardown.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			_ = rig.hub.Publish("alpha", "tick", i)
		}
	}()
	c.conn.Close()
	wg.Wait()

	waitFor(t, "subscription cleanup", func() bool { return !rig.hub.HasSubscribers("alpha") })
}

func TestRequestContextCarriesSessionAndIsCancelledOnDisconnect(t *testing.T) {
	logs := &syncBuffer{}
	logger := newBufferLogger(logs)

	path, restore := tempSocketPath(t)
	defer restore()

	got := make(chan context.Context, 1)
	server := NewWithContext(path, logger, func(ctx context.Context, msg []byte) ([]byte, error) {
		select {
		case got <- ctx:
		default:
		}
		return nil, nil
	})
	if err := server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer server.Shutdown()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := conn.Write([]byte("x\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	var ctx context.Context
	select {
	case ctx = <-got:
	case <-time.After(testIOTimeout):
		t.Fatal("handler not called")
	}
	sess, ok := SessionFromContext(ctx)
	if !ok || sess.ID() == 0 {
		t.Fatalf("context has no usable session: %v %v", sess, ok)
	}
	if ctx.Err() != nil {
		t.Fatal("context cancelled while the connection is open")
	}

	conn.Close()
	select {
	case <-ctx.Done():
	case <-time.After(testIOTimeout):
		t.Fatal("context not cancelled after disconnect")
	}
}

func TestNewStillAcceptsPlainHandler(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	d := jsonrpc.NewDispatcher()
	d.Register("ping", func(json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
		return json.RawMessage(`"pong"`), nil
	})

	logs := &syncBuffer{}
	server := New(path, newBufferLogger(logs), d.Handle)
	if err := server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer server.Shutdown()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	c := &testClient{t: t, conn: conn}
	c.reader = bufio.NewReader(conn)

	resp := c.call("ping", "")
	if string(resp.Result) != `"pong"` {
		t.Errorf("result = %s", resp.Result)
	}
}
