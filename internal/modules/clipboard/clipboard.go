package clipboard

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"omnibar/internal/modules"
)

const Name = "clipboard"
const ReadCapability = "clipboard.read"
const WriteCapability = "clipboard.write"

type Entry struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	CopiedAt string `json:"copied_at"`
}
type Snapshot struct {
	Revision   uint64  `json:"revision"`
	Collecting bool    `json:"collecting"`
	State      string  `json:"state"`
	Error      string  `json:"error,omitempty"`
	CanCopy    bool    `json:"can_copy"`
	Entries    []Entry `json:"entries"`
}
type Module struct {
	driver             Driver
	opMu               sync.Mutex
	cancel             context.CancelFunc
	done               chan struct{}
	running            bool
	mu                 sync.Mutex
	entries            []Entry
	totalBytes         int
	sequence, revision uint64
	collecting         bool
	state, errorText   string
}

func New(driver Driver) *Module {
	if driver == nil {
		driver = Wayland{}
	}
	return &Module{driver: driver, state: "paused", revision: 1, entries: []Entry{}}
}
func (m *Module) Info() modules.ModuleInfo {
	return modules.ModuleInfo{Name: Name, Description: "Session text clipboard history", Version: "0.1.0", Enabled: true, Capabilities: []string{ReadCapability, WriteCapability}}
}
func (m *Module) Initialize(context.Context) error { return nil }
func (m *Module) Start(context.Context) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.running = true
	return nil
}
func (m *Module) Stop(context.Context) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.stopWatcher()
	m.running = false
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = []Entry{}
	m.totalBytes = 0
	m.collecting = false
	m.state = "paused"
	m.errorText = ""
	m.revision++
	return nil
}
func (m *Module) Dispose(ctx context.Context) error { return m.Stop(ctx) }
func (m *Module) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := append([]Entry{}, m.entries...)
	return Snapshot{Revision: m.revision, Collecting: m.collecting, State: m.state, Error: m.errorText, Entries: entries}
}
func (m *Module) stopWatcher() {
	if m.cancel != nil {
		m.cancel()
		<-m.done
		m.cancel = nil
	}
}
func (m *Module) SetCollecting(active bool) (Snapshot, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if !m.running {
		return Snapshot{}, fmt.Errorf("clipboard module is unavailable")
	}
	if !active {
		m.stopWatcher()
		m.mu.Lock()
		m.collecting = false
		m.state = "paused"
		m.errorText = ""
		m.revision++
		m.mu.Unlock()
	} else if m.cancel == nil {
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		m.done = make(chan struct{})
		m.mu.Lock()
		m.collecting = true
		m.state = "watching"
		m.errorText = ""
		m.revision++
		m.mu.Unlock()
		go func() { defer close(m.done); m.watch(ctx) }()
	}
	return m.Snapshot(), nil
}
func (m *Module) watch(ctx context.Context) {
	for ctx.Err() == nil {
		err := m.driver.Watch(ctx, func(c Capture) {
			if ctx.Err() == nil {
				m.ingest(c)
			}
		})
		if ctx.Err() != nil {
			return
		}
		m.mu.Lock()
		m.state = "retrying"
		m.errorText = "Clipboard watcher disconnected; retrying."
		if err != nil {
			m.errorText = err.Error()
		}
		m.revision++
		m.mu.Unlock()
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func (m *Module) ingest(c Capture) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.collecting {
		return
	}
	if m.state != "watching" || m.errorText != "" {
		m.state = "watching"
		m.errorText = ""
		m.revision++
	}
	if c.State != "data" || !validText(c.Text) {
		return
	}
	var entry Entry
	for i, old := range m.entries {
		if old.Text == c.Text {
			entry = old
			m.totalBytes -= len(old.Text)
			copy(m.entries[i:], m.entries[i+1:])
			m.entries[len(m.entries)-1] = Entry{}
			m.entries = m.entries[:len(m.entries)-1]
			break
		}
	}
	if entry.ID == "" {
		m.sequence++
		entry.ID = strconv.FormatUint(m.sequence, 10)
	}
	entry.Text = c.Text
	entry.CopiedAt = time.Now().UTC().Format(time.RFC3339Nano)
	m.entries = append([]Entry{entry}, m.entries...)
	m.totalBytes += len(c.Text)
	for len(m.entries) > MaxEntries || m.totalBytes > MaxTotalBytes {
		last := len(m.entries) - 1
		m.totalBytes -= len(m.entries[last].Text)
		m.entries[last] = Entry{}
		m.entries = m.entries[:last]
	}
	m.revision++
}
func (m *Module) Delete(id string) (Snapshot, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	found := false
	for i, entry := range m.entries {
		if entry.ID == id {
			m.totalBytes -= len(entry.Text)
			copy(m.entries[i:], m.entries[i+1:])
			m.entries[len(m.entries)-1] = Entry{}
			m.entries = m.entries[:len(m.entries)-1]
			m.revision++
			found = true
			break
		}
	}
	m.mu.Unlock()
	if !found {
		return Snapshot{}, fmt.Errorf("history item is no longer available")
	}
	return m.Snapshot(), nil
}
func (m *Module) Clear() Snapshot {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	m.entries = []Entry{}
	m.totalBytes = 0
	m.revision++
	m.mu.Unlock()
	return m.Snapshot()
}
func (m *Module) Copy(ctx context.Context, id string) (Snapshot, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	text := ""
	for _, entry := range m.entries {
		if entry.ID == id {
			text = entry.Text
			break
		}
	}
	m.mu.Unlock()
	if text == "" {
		return Snapshot{}, fmt.Errorf("history item is no longer available")
	}
	if err := m.driver.Copy(ctx, text); err != nil {
		return Snapshot{}, err
	}
	return m.Snapshot(), nil
}
