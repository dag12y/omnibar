package audio

import (
	"context"
	"fmt"
	"sync"
	"time"

	"omnibar/internal/events"
	"omnibar/internal/modules"
)

const Name = "audio"
const Capability = "audio.control"

type Module struct {
	driver   Driver
	hub      *events.Hub
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	opMu     sync.Mutex
	revision uint64
}

func New(hub *events.Hub, driver Driver) *Module {
	if driver == nil {
		driver = NewPactl()
	}
	return &Module{hub: hub, driver: driver}
}
func (m *Module) Info() modules.ModuleInfo {
	return modules.ModuleInfo{Name: Name, Description: "Playback volume, mute, and default output", Version: "0.1.0", Enabled: true, Capabilities: []string{Capability}}
}
func (m *Module) Initialize(context.Context) error { return nil }
func (m *Module) Start(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.done = cancel, make(chan struct{})
	go func() { defer close(m.done); m.run(ctx) }()
	return nil
}
func (m *Module) Stop(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		<-m.done
		m.cancel = nil
	}
	return nil
}
func (m *Module) Dispose(ctx context.Context) error { return m.Stop(ctx) }
func (m *Module) readLocked(ctx context.Context) Snapshot {
	snap, err := m.driver.Read(ctx)
	if err != nil {
		snap = Snapshot{Error: err.Error()}
	}
	if snap.Outputs == nil {
		snap.Outputs = []Output{}
	}
	if snap.Streams == nil {
		snap.Streams = []Stream{}
	}
	m.revision++
	snap.Revision = m.revision
	return snap
}
func (m *Module) publish(snap Snapshot) {
	if m.hub != nil {
		_ = m.hub.Publish(Name, "snapshot", snap)
	}
}
func (m *Module) Snapshot(ctx context.Context) Snapshot {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	return m.readLocked(ctx)
}
func (m *Module) refresh(ctx context.Context) {
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	m.opMu.Lock()
	defer m.opMu.Unlock()
	snap := m.readLocked(ctx)
	if parent.Err() == nil {
		m.publish(snap)
	}
}

// Change validates against a fresh server snapshot. Stream IDs and output names
// come from that snapshot, never from a shell command supplied by a client.
func (m *Module) Change(ctx context.Context, action, key string, volume int, muted bool) (Snapshot, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	snap := m.readLocked(ctx)
	if !snap.Available {
		return snap, fmt.Errorf("audio service unavailable: %s", snap.Error)
	}
	var err error
	if action == "output" {
		var name string
		for _, output := range snap.Outputs {
			if output.Name == key {
				name = output.Name
				break
			}
		}
		if name == "" {
			return snap, fmt.Errorf("output is no longer available; refresh and try again")
		}
		err = m.driver.SetDefaultOutput(ctx, name)
	} else {
		var id string
		for _, stream := range snap.Streams {
			if stream.Key == key {
				id = stream.ID
				break
			}
		}
		if id == "" {
			return snap, fmt.Errorf("playback stream ended or changed; refresh and try again")
		}
		switch action {
		case "volume":
			if volume < 0 || volume > 100 {
				return snap, fmt.Errorf("volume must be between 0 and 100")
			}
			err = m.driver.SetVolume(ctx, id, volume)
		case "mute":
			err = m.driver.SetMute(ctx, id, muted)
		default:
			return snap, fmt.Errorf("unknown audio action")
		}
	}
	if err != nil {
		return snap, err
	}
	snap = m.readLocked(ctx)
	if !snap.Available {
		return snap, fmt.Errorf("audio changed but refresh failed: %s", snap.Error)
	}
	m.publish(snap)
	return snap, nil
}
func (m *Module) run(ctx context.Context) {
	if m.hub == nil {
		<-ctx.Done()
		return
	}
	demand := m.hub.Watch(Name)
	for ctx.Err() == nil {
		if !m.hub.HasSubscribers(Name) {
			select {
			case <-ctx.Done():
				return
			case <-demand:
			}
			continue
		}
		watchCtx, cancel := context.WithCancel(ctx)
		changed := make(chan struct{}, 1)
		watchDone := make(chan error, 1)
		go func() { watchDone <- m.driver.Watch(watchCtx, changed) }()
		m.refresh(watchCtx)
		resync := time.NewTicker(3 * time.Second)
		debounce := time.NewTimer(time.Hour)
		debounce.Stop()
		var pending <-chan time.Time
		watching := true
		for watching {
			select {
			case <-ctx.Done():
				watching = false
			case <-demand:
				watching = m.hub.HasSubscribers(Name)
			case <-changed:
				if pending == nil {
					debounce.Reset(100 * time.Millisecond)
					pending = debounce.C
				}
			case <-pending:
				pending = nil
				m.refresh(watchCtx)
			case <-resync.C:
				m.refresh(watchCtx)
			case <-watchDone:
				// Reconnect without restarting Omnibar after a server restart. A read
				// reports the current unavailable state while the service is down.
				watchDone = nil
				m.refresh(watchCtx)
				watching = false
			}
		}
		debounce.Stop()
		resync.Stop()
		cancel()
		if watchDone != nil {
			<-watchDone
		}
		if ctx.Err() != nil {
			return
		}
		if m.hub.HasSubscribers(Name) {
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-demand:
				timer.Stop()
			case <-timer.C:
			}
		}
	}
}
