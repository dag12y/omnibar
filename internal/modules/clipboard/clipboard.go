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
	Favorite bool   `json:"favorite"`
}
type Snapshot struct {
	Revision       uint64  `json:"revision"`
	Collecting     bool    `json:"collecting"`
	State          string  `json:"state"`
	Error          string  `json:"error,omitempty"`
	CanFavorite    bool    `json:"can_favorite"`
	FavoritesError string  `json:"favorites_error,omitempty"`
	CanCopy        bool    `json:"can_copy"`
	Entries        []Entry `json:"entries"`
}
type Module struct {
	store              FavoritesStore
	favoritesError     string
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

func New(driver Driver) *Module { return NewWithStore(driver, nil) }

func NewWithStore(driver Driver, store FavoritesStore) *Module {
	if driver == nil {
		driver = Wayland{}
	}
	return &Module{driver: driver, store: store, state: "paused", revision: 1, entries: []Entry{}}
}
func (m *Module) Info() modules.ModuleInfo {
	return modules.ModuleInfo{Name: Name, Description: "Text clipboard history and saved favorites", Version: "0.1.0", Enabled: true, Capabilities: []string{ReadCapability, WriteCapability}}
}
func (m *Module) Initialize(context.Context) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store == nil {
		return nil
	}
	entries, err := m.store.Load()
	if err == nil {
		err = validateFavorites(entries)
	}
	if err != nil {
		m.favoritesError = "Could not load favorites. " + err.Error() + ". Fix the saved file and restart; it has not been overwritten."
		return nil
	}
	for _, entry := range entries {
		m.sequence++
		entry.ID = strconv.FormatUint(m.sequence, 10)
		entry.Favorite = true
		m.entries = append(m.entries, entry)
		m.totalBytes += len(entry.Text)
	}
	return nil
}
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
	entries := make([]Entry, 0, len(m.entries))
	for _, favorite := range []bool{true, false} {
		for _, entry := range m.entries {
			if entry.Favorite == favorite {
				entries = append(entries, entry)
			}
		}
	}
	return Snapshot{Revision: m.revision, Collecting: m.collecting, State: m.state, Error: m.errorText, CanFavorite: m.store != nil && m.favoritesError == "", FavoritesError: m.favoritesError, Entries: entries}
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
		for last >= 0 && m.entries[last].Favorite {
			last--
		}
		if last < 0 {
			break
		}
		m.totalBytes -= len(m.entries[last].Text)
		copy(m.entries[last:], m.entries[last+1:])
		m.entries[len(m.entries)-1] = Entry{}
		m.entries = m.entries[:len(m.entries)-1]
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
			if entry.Favorite {
				if err := m.saveFavoritesLocked(id, false); err != nil {
					m.mu.Unlock()
					return Snapshot{}, err
				}
			}
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
	kept := make([]Entry, 0)
	m.totalBytes = 0
	for _, entry := range m.entries {
		if entry.Favorite {
			kept = append(kept, entry)
			m.totalBytes += len(entry.Text)
		}
	}
	m.entries = kept
	m.revision++
	m.mu.Unlock()
	return m.Snapshot()
}
func (m *Module) Copy(ctx context.Context, id string) (Snapshot, error) {
	return m.copyText(ctx, id, "")
}

// TransformCopy copies a derived value without editing history or saved text.
func (m *Module) TransformCopy(ctx context.Context, id, operation string) (Snapshot, error) {
	if !IsTransform(operation) {
		return Snapshot{}, fmt.Errorf("unsupported clipboard transform")
	}
	return m.copyText(ctx, id, operation)
}
func (m *Module) copyText(ctx context.Context, id, operation string) (Snapshot, error) {
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
	if operation != "" {
		var err error
		text, err = transformText(text, operation)
		if err != nil {
			return Snapshot{}, err
		}
	}
	if err := m.driver.Copy(ctx, text); err != nil {
		return Snapshot{}, err
	}
	return m.Snapshot(), nil
}

// saveFavoritesLocked writes the proposed change before changing visible state.
// Both opMu and mu are held so ingestion cannot invalidate the saved entry.
func (m *Module) saveFavoritesLocked(id string, favorite bool) error {
	if m.store == nil {
		return fmt.Errorf("favorites storage is unavailable")
	}
	if m.favoritesError != "" {
		return fmt.Errorf("fix favorites storage and restart before changing saved items")
	}
	saved := make([]Entry, 0)
	for _, entry := range m.entries {
		keep := entry.Favorite
		if entry.ID == id {
			keep = favorite
		}
		if keep {
			saved = append(saved, entry)
		}
	}
	if err := validateFavorites(saved); err != nil {
		return err
	}
	return m.store.Save(saved)
}
func (m *Module) SetFavorite(id string, favorite bool) (Snapshot, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	found := false
	for i, entry := range m.entries {
		if entry.ID != id {
			continue
		}
		found = true
		if entry.Favorite != favorite {
			if err := m.saveFavoritesLocked(id, favorite); err != nil {
				m.mu.Unlock()
				return Snapshot{}, err
			}
			m.entries[i].Favorite = favorite
			m.revision++
		}
		break
	}
	m.mu.Unlock()
	if !found {
		return Snapshot{}, fmt.Errorf("history item is no longer available")
	}
	return m.Snapshot(), nil
}
