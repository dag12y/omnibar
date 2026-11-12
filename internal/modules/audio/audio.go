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
	return modules.ModuleInfo{Name: Name, Description: "Playback, microphone controls, and recording-app metadata", Version: "0.1.0", Enabled: true, Capabilities: []string{Capability}}
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
	if snap.Inputs == nil {
		snap.Inputs = []Input{}
	}
	if snap.Recordings == nil {
		snap.Recordings = []Recording{}
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
	return m.mutate(ctx, func(snap Snapshot) error {
		if action == "output" {
			output, err := findOutput(snap, key)
			if err != nil {
				return err
			}
			return m.driver.SetDefaultOutput(ctx, output.Name)
		}
		stream, err := findStream(snap, key)
		if err != nil {
			return err
		}
		switch action {
		case "volume":
			if volume < 0 || volume > 100 {
				return fmt.Errorf("volume must be between 0 and 100")
			}
			return m.driver.SetVolume(ctx, stream.ID, volume)
		case "mute":
			return m.driver.SetMute(ctx, stream.ID, muted)
		default:
			return fmt.Errorf("unknown audio action")
		}
	})
}

// MoveStream moves one currently existing playback stream. It does not change
// the default output, other streams, mute, or volume, and stores no preference.
func (m *Module) MoveStream(ctx context.Context, key, outputName string) (Snapshot, error) {
	return m.mutate(ctx, func(snap Snapshot) error {
		stream, err := findStream(snap, key)
		if err != nil {
			return err
		}
		output, err := findOutput(snap, outputName)
		if err != nil {
			return err
		}
		return m.driver.MoveStream(ctx, stream.ID, output.Name)
	})
}

// ChangeInput changes a named, currently connected non-monitor source. It
// never opens a recording stream or changes capture-stream routing.
func (m *Module) ChangeInput(ctx context.Context, action, name string, volume int, muted bool) (Snapshot, error) {
	return m.mutate(ctx, func(snap Snapshot) error {
		var input *Input
		for i := range snap.Inputs {
			if snap.Inputs[i].Name == name {
				input = &snap.Inputs[i]
				break
			}
		}
		if input == nil {
			return fmt.Errorf("microphone input is no longer available; refresh and try again")
		}
		if input.Monitor {
			return fmt.Errorf("choose a microphone input, not an output monitor")
		}
		switch action {
		case "input-volume":
			if volume < 0 || volume > 100 {
				return fmt.Errorf("microphone gain must be between 0 and 100")
			}
			return m.driver.SetInputVolume(ctx, input.Name, volume)
		case "input-mute":
			return m.driver.SetInputMute(ctx, input.Name, muted)
		case "input-default":
			return m.driver.SetDefaultInput(ctx, input.Name)
		default:
			return fmt.Errorf("unknown microphone action")
		}
	})
}
func findStream(snap Snapshot, key string) (Stream, error) {
	for _, stream := range snap.Streams {
		if stream.Key == key {
			return stream, nil
		}
	}
	return Stream{}, fmt.Errorf("playback stream ended or changed; refresh and try again")
}
func findOutput(snap Snapshot, name string) (Output, error) {
	for _, output := range snap.Outputs {
		if output.Name == name {
			return output, nil
		}
	}
	return Output{}, fmt.Errorf("output is no longer available; refresh and try again")
}
func (m *Module) mutate(ctx context.Context, change func(Snapshot) error) (Snapshot, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	snap := m.readLocked(ctx)
	if !snap.Available {
		return snap, fmt.Errorf("audio service unavailable: %s", snap.Error)
	}
	if err := change(snap); err != nil {
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
