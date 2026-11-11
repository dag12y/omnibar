// Package events implements Omnibar's in-process event hub (ADR-0009).
//
// Modules publish events to the hub, and the IPC layer subscribes to it on
// behalf of each client connection. Modules never hold connections or call
// each other directly: everything flows through the hub.
//
// Delivery is at most once with no replay. Publish never blocks on a
// subscriber; a Subscriber that cannot accept an event must return false
// from Deliver without waiting, and is responsible for its own recovery
// (the IPC session disconnects the slow client).
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
)

// All is the event filter that matches every event of a module. It is
// reserved: no module may publish an event named "*".
const All = "*"

// ErrModuleNotRunning is returned by Publisher.Publish when the owning
// module is not running. Publishing from a stopped module is a no-op.
var ErrModuleNotRunning = errors.New("module is not running")

// Event is a single published event.
type Event struct {
	Module string
	Name   string
	Data   json.RawMessage
}

// Subscriber receives events matching its subscriptions.
//
// Deliver must not block. It returns false when the event could not be
// accepted (for example a full outbound queue). The hub only logs that;
// the subscriber decides what to do about it.
type Subscriber interface {
	Deliver(Event) bool
}

// filter is one subscriber's event filter for one module.
type filter struct {
	all   bool
	names map[string]struct{}
}

func (f *filter) matches(name string) bool {
	if f.all {
		return true
	}
	_, ok := f.names[name]
	return ok
}

// list returns the filter as a stable list: ["*"] for all events,
// otherwise the sorted event names.
func (f *filter) list() []string {
	if f.all {
		return []string{All}
	}
	out := make([]string, 0, len(f.names))
	for name := range f.names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Hub routes published events to subscribers. It is safe for concurrent
// use.
type Hub struct {
	logger *slog.Logger

	mu       sync.RWMutex
	subs     map[string]map[Subscriber]*filter
	watchers map[string][]chan struct{}
}

// New creates an empty hub. A nil logger discards hub log output.
func New(logger *slog.Logger) *Hub {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Hub{
		logger:   logger,
		subs:     make(map[string]map[Subscriber]*filter),
		watchers: make(map[string][]chan struct{}),
	}
}

// Subscribe adds events for module to s's subscription. A nil or empty
// events list subscribes to every event. Subscribing is additive and
// idempotent, and a wildcard subscription absorbs named ones. It returns
// the resulting filter: ["*"] or the sorted event names.
func (h *Hub) Subscribe(s Subscriber, module string, events []string) []string {
	h.mu.Lock()
	byModule := h.subs[module]
	if byModule == nil {
		byModule = make(map[Subscriber]*filter)
		h.subs[module] = byModule
	}
	before := len(byModule)

	f := byModule[s]
	if f == nil {
		f = &filter{names: make(map[string]struct{})}
		byModule[s] = f
	}
	if len(events) == 0 {
		f.all = true
		f.names = make(map[string]struct{})
	} else if !f.all {
		for _, name := range events {
			f.names[name] = struct{}{}
		}
	}
	result := f.list()
	watchers := h.watchersIfChanged(module, before)
	h.mu.Unlock()

	signal(watchers)
	return result
}

// Unsubscribe removes events for module from s's subscription. A nil or
// empty events list removes the whole module subscription. Removing
// something that is not subscribed is not an error. Named events cannot
// be carved out of a wildcard subscription; only removing the whole module
// ends it. It returns the remaining filter ([] when nothing remains).
func (h *Hub) Unsubscribe(s Subscriber, module string, events []string) []string {
	h.mu.Lock()
	byModule := h.subs[module]
	f := byModule[s]
	if f == nil {
		h.mu.Unlock()
		return []string{}
	}
	before := len(byModule)

	if len(events) == 0 {
		delete(byModule, s)
	} else if !f.all {
		for _, name := range events {
			delete(f.names, name)
		}
		if len(f.names) == 0 {
			delete(byModule, s)
		}
	}

	var result []string
	if remaining, ok := byModule[s]; ok {
		result = remaining.list()
	} else {
		result = []string{}
	}
	if len(byModule) == 0 {
		delete(h.subs, module)
	}
	watchers := h.watchersIfChanged(module, before)
	h.mu.Unlock()

	signal(watchers)
	return result
}

// Remove drops every subscription held by s. The IPC layer calls it when
// a connection closes.
func (h *Hub) Remove(s Subscriber) {
	h.mu.Lock()
	var notify [][]chan struct{}
	for module, byModule := range h.subs {
		if _, ok := byModule[s]; !ok {
			continue
		}
		before := len(byModule)
		delete(byModule, s)
		if len(byModule) == 0 {
			delete(h.subs, module)
		}
		notify = append(notify, h.watchersIfChanged(module, before))
	}
	h.mu.Unlock()

	for _, watchers := range notify {
		signal(watchers)
	}
}

// HasSubscribers reports whether any subscriber is subscribed to module.
func (h *Hub) HasSubscribers(module string) bool {
	return h.SubscriberCount(module) > 0
}

// SubscriberCount returns how many subscribers are subscribed to module.
func (h *Hub) SubscriberCount(module string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[module])
}

// Watch returns a channel that receives a value whenever module's
// subscriber count changes between none and some. The channel has a
// buffer of one and sends never block, so rapid changes coalesce: after a
// receive, call HasSubscribers to learn the current state.
func (h *Hub) Watch(module string) <-chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.watchers[module] = append(h.watchers[module], ch)
	h.mu.Unlock()
	return ch
}

// watchersIfChanged returns module's watchers when its subscriber count
// moved between none and some. before is the subscriber count prior to the
// change. Must be called with h.mu held.
func (h *Hub) watchersIfChanged(module string, before int) []chan struct{} {
	after := len(h.subs[module])
	if (before == 0) == (after == 0) {
		return nil
	}
	return append([]chan struct{}(nil), h.watchers[module]...)
}

func signal(watchers []chan struct{}) {
	for _, ch := range watchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Publish delivers an event to every subscriber of module whose filter
// matches name. data is marshaled to JSON once. Publish never blocks on a
// subscriber and recovers from panics in Subscriber.Deliver.
func (h *Hub) Publish(module, name string, data any) error {
	if module == "" {
		return errors.New("events: module name is required")
	}
	if name == "" {
		return errors.New("events: event name is required")
	}
	if name == All {
		return fmt.Errorf("events: event name %q is reserved", All)
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("events: marshal %s/%s data: %w", module, name, err)
	}
	ev := Event{Module: module, Name: name, Data: raw}

	h.mu.RLock()
	byModule := h.subs[module]
	targets := make([]Subscriber, 0, len(byModule))
	for s, f := range byModule {
		if f.matches(name) {
			targets = append(targets, s)
		}
	}
	h.mu.RUnlock()

	for _, s := range targets {
		h.deliver(s, ev)
	}
	return nil
}

func (h *Hub) deliver(s Subscriber, ev Event) {
	defer func() {
		if r := recover(); r != nil {
			h.logger.Error("subscriber panicked during delivery",
				"module", ev.Module, "event", ev.Name, "panic", r)
		}
	}()
	if !s.Deliver(ev) {
		h.logger.Debug("subscriber did not accept event", "module", ev.Module, "event", ev.Name)
	}
}

// Publisher lets one module publish events under its own name. Publishing
// is a no-op while the module is not running.
type Publisher struct {
	hub     *Hub
	module  string
	running func() bool
}

// Publisher returns a publisher for module. running reports whether the
// module is currently running; a nil running function means always.
func (h *Hub) Publisher(module string, running func() bool) *Publisher {
	return &Publisher{hub: h, module: module, running: running}
}

// Publish publishes name with data as the owning module. It returns
// ErrModuleNotRunning, without publishing, when the module is not running.
func (p *Publisher) Publish(name string, data any) error {
	if p.running != nil && !p.running() {
		p.hub.logger.Debug("dropping event from module that is not running",
			"module", p.module, "event", name)
		return ErrModuleNotRunning
	}
	return p.hub.Publish(p.module, name, data)
}
