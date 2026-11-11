package events

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recorder is a Subscriber that records every event it accepts.
type recorder struct {
	mu     sync.Mutex
	events []Event
	accept bool
}

func newRecorder() *recorder { return &recorder{accept: true} }

func (r *recorder) Deliver(ev Event) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.accept {
		return false
	}
	r.events = append(r.events, ev)
	return true
}

func (r *recorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.events))
	for _, ev := range r.events {
		out = append(out, ev.Module+"/"+ev.Name)
	}
	return out
}

func TestSubscribeAllReceivesEveryEvent(t *testing.T) {
	h := New(nil)
	r := newRecorder()

	if got := h.Subscribe(r, "m", nil); !reflect.DeepEqual(got, []string{All}) {
		t.Fatalf("Subscribe filter = %v, want [*]", got)
	}
	for _, name := range []string{"a", "b", "c"} {
		if err := h.Publish("m", name, nil); err != nil {
			t.Fatalf("Publish(%s): %v", name, err)
		}
	}

	want := []string{"m/a", "m/b", "m/c"}
	if got := r.names(); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v (publish order)", got, want)
	}
}

func TestSubscribeNamedEventsFilters(t *testing.T) {
	h := New(nil)
	r := newRecorder()

	got := h.Subscribe(r, "m", []string{"b", "a"})
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("Subscribe filter = %v, want sorted [a b]", got)
	}
	for _, name := range []string{"a", "b", "c"} {
		_ = h.Publish("m", name, nil)
	}

	if got, want := r.names(), []string{"m/a", "m/b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestSubscribeIsAdditiveAndIdempotent(t *testing.T) {
	h := New(nil)
	r := newRecorder()

	h.Subscribe(r, "m", []string{"a"})
	h.Subscribe(r, "m", []string{"a"})
	got := h.Subscribe(r, "m", []string{"b"})
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("filter after additive subscribe = %v, want [a b]", got)
	}

	_ = h.Publish("m", "a", nil)
	if n := len(r.names()); n != 1 {
		t.Errorf("duplicate subscribe delivered %d copies, want 1", n)
	}
}

func TestWildcardAbsorbsNamedEvents(t *testing.T) {
	h := New(nil)
	r := newRecorder()

	h.Subscribe(r, "m", []string{"a"})
	got := h.Subscribe(r, "m", nil)
	if !reflect.DeepEqual(got, []string{All}) {
		t.Fatalf("filter = %v, want [*]", got)
	}
	// Further named subscriptions leave the wildcard in place.
	if got := h.Subscribe(r, "m", []string{"b"}); !reflect.DeepEqual(got, []string{All}) {
		t.Errorf("filter after named subscribe on wildcard = %v, want [*]", got)
	}
	_ = h.Publish("m", "zzz", nil)
	if n := len(r.names()); n != 1 {
		t.Errorf("wildcard subscriber got %d events, want 1", n)
	}
}

func TestEventsAreScopedToModule(t *testing.T) {
	h := New(nil)
	r := newRecorder()
	h.Subscribe(r, "m1", nil)

	_ = h.Publish("m2", "x", nil)
	if n := len(r.names()); n != 0 {
		t.Errorf("subscriber of m1 received %d events from m2", n)
	}
}

func TestUnsubscribe(t *testing.T) {
	t.Run("named events", func(t *testing.T) {
		h := New(nil)
		r := newRecorder()
		h.Subscribe(r, "m", []string{"a", "b"})

		got := h.Unsubscribe(r, "m", []string{"a"})
		if !reflect.DeepEqual(got, []string{"b"}) {
			t.Fatalf("remaining = %v, want [b]", got)
		}
		_ = h.Publish("m", "a", nil)
		_ = h.Publish("m", "b", nil)
		if got, want := r.names(), []string{"m/b"}; !reflect.DeepEqual(got, want) {
			t.Errorf("events = %v, want %v", got, want)
		}
	})

	t.Run("last named event removes the module", func(t *testing.T) {
		h := New(nil)
		r := newRecorder()
		h.Subscribe(r, "m", []string{"a"})

		got := h.Unsubscribe(r, "m", []string{"a"})
		if len(got) != 0 {
			t.Errorf("remaining = %v, want []", got)
		}
		if h.HasSubscribers("m") {
			t.Error("module still has subscribers")
		}
	})

	t.Run("whole module", func(t *testing.T) {
		h := New(nil)
		r := newRecorder()
		h.Subscribe(r, "m", nil)

		if got := h.Unsubscribe(r, "m", nil); len(got) != 0 {
			t.Errorf("remaining = %v, want []", got)
		}
		_ = h.Publish("m", "a", nil)
		if n := len(r.names()); n != 0 {
			t.Errorf("received %d events after unsubscribe", n)
		}
	})

	t.Run("named events cannot be carved out of a wildcard", func(t *testing.T) {
		h := New(nil)
		r := newRecorder()
		h.Subscribe(r, "m", nil)

		got := h.Unsubscribe(r, "m", []string{"a"})
		if !reflect.DeepEqual(got, []string{All}) {
			t.Errorf("remaining = %v, want [*]", got)
		}
	})

	t.Run("not subscribed is not an error", func(t *testing.T) {
		h := New(nil)
		r := newRecorder()
		if got := h.Unsubscribe(r, "m", nil); got == nil || len(got) != 0 {
			t.Errorf("remaining = %#v, want empty non-nil slice", got)
		}
	})
}

func TestRemoveDropsAllSubscriptions(t *testing.T) {
	h := New(nil)
	r := newRecorder()
	other := newRecorder()
	h.Subscribe(r, "m1", nil)
	h.Subscribe(r, "m2", []string{"x"})
	h.Subscribe(other, "m1", nil)

	h.Remove(r)

	if h.SubscriberCount("m1") != 1 || h.SubscriberCount("m2") != 0 {
		t.Errorf("counts after Remove: m1=%d m2=%d, want 1 and 0",
			h.SubscriberCount("m1"), h.SubscriberCount("m2"))
	}
	_ = h.Publish("m1", "a", nil)
	if len(r.names()) != 0 || len(other.names()) != 1 {
		t.Errorf("removed=%v other=%v", r.names(), other.names())
	}

	// Removing a subscriber that is not present is safe.
	h.Remove(newRecorder())
}

func TestPublishDataIsJSON(t *testing.T) {
	h := New(nil)
	r := newRecorder()
	h.Subscribe(r, "m", nil)

	if err := h.Publish("m", "cpu", map[string]any{"percent": 4.2}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := h.Publish("m", "none", nil); err != nil {
		t.Fatalf("Publish nil data: %v", err)
	}

	if got := string(r.events[0].Data); got != `{"percent":4.2}` {
		t.Errorf("data = %s", got)
	}
	if got := string(r.events[1].Data); got != "null" {
		t.Errorf("nil data = %s, want null", got)
	}
}

func TestPublishValidation(t *testing.T) {
	h := New(nil)

	cases := []struct {
		name         string
		module, ev   string
		data         any
		wantContains string
	}{
		{"empty module", "", "e", nil, "module name"},
		{"empty event", "m", "", nil, "event name"},
		{"reserved event", "m", All, nil, "reserved"},
		{"unmarshalable data", "m", "e", make(chan int), "marshal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := h.Publish(tc.module, tc.ev, tc.data)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !contains(err.Error(), tc.wantContains) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantContains)
			}
		})
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestPublishWithNoSubscribersIsNotAnError(t *testing.T) {
	if err := New(nil).Publish("m", "e", 1); err != nil {
		t.Fatalf("Publish with no subscribers: %v", err)
	}
}

type panicker struct{}

func (panicker) Deliver(Event) bool { panic("boom") }

func TestPanickingSubscriberDoesNotAffectOthers(t *testing.T) {
	h := New(nil)
	good := newRecorder()
	h.Subscribe(panicker{}, "m", nil)
	h.Subscribe(good, "m", nil)

	if err := h.Publish("m", "e", nil); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if n := len(good.names()); n != 1 {
		t.Errorf("healthy subscriber got %d events, want 1", n)
	}
}

func TestRejectedDeliveryDoesNotFailPublish(t *testing.T) {
	h := New(nil)
	r := newRecorder()
	r.accept = false
	h.Subscribe(r, "m", nil)

	if err := h.Publish("m", "e", nil); err != nil {
		t.Fatalf("Publish: %v", err)
	}
}

func TestWatchSignalsDemandTransitions(t *testing.T) {
	h := New(nil)
	watch := h.Watch("m")

	expectSignal := func(t *testing.T, step string) {
		t.Helper()
		select {
		case <-watch:
		case <-time.After(time.Second):
			t.Fatalf("%s: no signal", step)
		}
	}
	expectNoSignal := func(t *testing.T, step string) {
		t.Helper()
		select {
		case <-watch:
			t.Fatalf("%s: unexpected signal", step)
		case <-time.After(50 * time.Millisecond):
		}
	}

	a, b := newRecorder(), newRecorder()

	h.Subscribe(a, "m", nil)
	expectSignal(t, "first subscriber")

	h.Subscribe(b, "m", nil)
	expectNoSignal(t, "second subscriber")

	h.Subscribe(a, "m", []string{"x"})
	expectNoSignal(t, "repeat subscribe")

	h.Unsubscribe(a, "m", nil)
	expectNoSignal(t, "one subscriber remains")

	h.Remove(b)
	expectSignal(t, "last subscriber removed")

	// Other modules do not signal this watcher.
	h.Subscribe(a, "other", nil)
	expectNoSignal(t, "other module")
}

func TestWatchCoalesces(t *testing.T) {
	h := New(nil)
	watch := h.Watch("m")
	r := newRecorder()

	// Several transitions with nobody receiving must never block.
	for i := 0; i < 10; i++ {
		h.Subscribe(r, "m", nil)
		h.Remove(r)
	}

	select {
	case <-watch:
	default:
		t.Fatal("expected a pending signal")
	}
	select {
	case <-watch:
		t.Fatal("signals should coalesce into one")
	default:
	}
}

func TestPublisherDropsWhenModuleNotRunning(t *testing.T) {
	h := New(nil)
	r := newRecorder()
	h.Subscribe(r, "m", nil)

	var running atomic.Bool
	p := h.Publisher("m", running.Load)

	if err := p.Publish("e", nil); !errors.Is(err, ErrModuleNotRunning) {
		t.Fatalf("Publish while stopped = %v, want ErrModuleNotRunning", err)
	}
	if n := len(r.names()); n != 0 {
		t.Fatalf("stopped module delivered %d events", n)
	}

	running.Store(true)
	if err := p.Publish("e", nil); err != nil {
		t.Fatalf("Publish while running: %v", err)
	}
	if got, want := r.names(), []string{"m/e"}; !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestPublisherNilRunningMeansAlways(t *testing.T) {
	h := New(nil)
	r := newRecorder()
	h.Subscribe(r, "m", nil)

	if err := h.Publisher("m", nil).Publish("e", nil); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(r.names()) != 1 {
		t.Error("event was not delivered")
	}
}

func TestEventDataSurvivesJSONRoundTrip(t *testing.T) {
	h := New(nil)
	r := newRecorder()
	h.Subscribe(r, "m", nil)
	_ = h.Publish("m", "e", map[string][]int{"values": {1, 2, 3}})

	var got map[string][]int
	if err := json.Unmarshal(r.events[0].Data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got["values"], []int{1, 2, 3}) {
		t.Errorf("values = %v", got["values"])
	}
}

// TestConcurrentUseIsRaceFree exercises every operation from many
// goroutines; it is meaningful under `go test -race`.
func TestConcurrentUseIsRaceFree(t *testing.T) {
	h := New(nil)
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := newRecorder()
			for j := 0; j < 200; j++ {
				h.Subscribe(r, "m", []string{"a"})
				h.Subscribe(r, "m", nil)
				_ = h.HasSubscribers("m")
				h.Unsubscribe(r, "m", nil)
				h.Remove(r)
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 400; j++ {
				_ = h.Publish("m", "a", j)
			}
		}()
	}
	watch := h.Watch("m")
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-watch:
			case <-done:
				return
			}
		}
	}()

	wg.Wait()
	close(done)

	if n := h.SubscriberCount("m"); n != 0 {
		t.Errorf("subscribers left after all removed: %d", n)
	}
}
