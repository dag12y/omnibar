// Package keepawake owns a timed logind idle/sleep inhibitor. Sessions belong
// to the backend, so closing a panel or disconnecting a client does not end them.
package keepawake

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"omnibar/internal/events"
	"omnibar/internal/modules"
)

const Name = "keep_awake"
const Capability = "power.inhibit"
const MaxSeconds = 24 * 60 * 60

type Status struct {
	Active    bool   `json:"active"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type AcquireFunc func(context.Context) (io.Closer, error)

type Module struct {
	mu         sync.Mutex
	acquire    AcquireFunc
	hub        *events.Hub
	running    bool
	inhibitor  io.Closer
	timer      *time.Timer
	generation uint64
	expires    time.Time
	reason     string
}

func New(hub *events.Hub, acquire AcquireFunc) *Module {
	if acquire == nil {
		acquire = acquireLogind
	}
	return &Module{hub: hub, acquire: acquire}
}

func (m *Module) Info() modules.ModuleInfo {
	return modules.ModuleInfo{Name: Name, Description: "Timed keep awake", Version: "0.1.0", Enabled: true, Capabilities: []string{Capability}}
}
func (m *Module) Initialize(context.Context) error { return nil }
func (m *Module) Start(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = true
	return nil
}
func (m *Module) Stop(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	return m.endLocked("shutdown")
}
func (m *Module) Dispose(ctx context.Context) error { return m.Stop(ctx) }

func (m *Module) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statusLocked()
}
func (m *Module) statusLocked() Status {
	status := Status{Active: m.inhibitor != nil, Reason: m.reason}
	if status.Active {
		status.ExpiresAt = m.expires.UTC().Format(time.RFC3339Nano)
	}
	return status
}
func (m *Module) publishLocked() {
	if m.hub != nil {
		_ = m.hub.Publish(Name, "state", m.statusLocked())
	}
}

// Begin acquires the inhibitor before reporting success. Repeated starts extend
// the existing session; failure leaves no timer or active flag behind.
func (m *Module) Begin(ctx context.Context, seconds int) (Status, error) {
	if seconds < 1 || seconds > MaxSeconds {
		return Status{}, fmt.Errorf("seconds must be between 1 and %d", MaxSeconds)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return Status{}, fmt.Errorf("keep awake is unavailable")
	}
	if m.inhibitor == nil {
		lease, err := m.acquire(ctx)
		if err != nil {
			return Status{}, fmt.Errorf("could not inhibit idle/sleep: %w", err)
		}
		if lease == nil {
			return Status{}, fmt.Errorf("inhibitor acquisition returned no lease")
		}
		m.inhibitor = lease
	}
	if m.timer != nil {
		m.timer.Stop()
	}
	m.generation++
	generation := m.generation
	duration := time.Duration(seconds) * time.Second
	m.expires = time.Now().Add(duration)
	m.reason = ""
	m.timer = time.AfterFunc(duration, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if generation == m.generation {
			_ = m.endLocked("expired")
		}
	})
	m.publishLocked()
	return m.statusLocked(), nil
}
func (m *Module) End() (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.endLocked("stopped")
	return m.statusLocked(), err
}
func (m *Module) endLocked(reason string) error {
	m.generation++
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	var err error
	if m.inhibitor != nil {
		err = m.inhibitor.Close()
		m.inhibitor = nil
	}
	m.expires = time.Time{}
	m.reason = reason
	m.publishLocked()
	return err
}
