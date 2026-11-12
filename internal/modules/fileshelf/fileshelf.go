// Package fileshelf keeps bounded session references to explicitly added local
// files and folders. It never reads, copies, moves, or deletes their contents.
package fileshelf

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"omnibar/internal/modules"
)

const Name = "file_shelf"
const Capability = "filesystem.access"
const CopyCapability = "clipboard.write"
const MaxEntries = 50
const MaxPathBytes = 4096
const MaxTotalPathBytes = 32 * 1024

type Entry struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	Directory bool   `json:"directory"`
	Available bool   `json:"available"`
	Size      int64  `json:"size"`
}
type Snapshot struct {
	Revision uint64  `json:"revision"`
	CanCopy  bool    `json:"can_copy"`
	Entries  []Entry `json:"entries"`
}
type Module struct {
	mu                 sync.Mutex
	driver             Driver
	entries            []Entry
	sequence, revision uint64
}

func New(driver Driver) *Module {
	if driver == nil {
		driver = Desktop{}
	}
	return &Module{driver: driver, entries: []Entry{}, revision: 1}
}
func (m *Module) Info() modules.ModuleInfo {
	return modules.ModuleInfo{Name: Name, Description: "Session references to local files and folders", Version: "0.1.0", Enabled: true, Capabilities: []string{Capability, CopyCapability}}
}
func (m *Module) Initialize(context.Context) error  { return nil }
func (m *Module) Start(context.Context) error       { return nil }
func (m *Module) Stop(context.Context) error        { m.Clear(); return nil }
func (m *Module) Dispose(ctx context.Context) error { return m.Stop(ctx) }

func localPath(value string) (string, error) {
	if !utf8.ValidString(value) || len(value) > MaxPathBytes*3+16 {
		return "", fmt.Errorf("invalid or oversized local path")
	}
	path := value
	if strings.HasPrefix(value, "file:") {
		uri, err := url.Parse(value)
		if err != nil || uri.Scheme != "file" || uri.Opaque != "" || (uri.Host != "" && uri.Host != "localhost") || uri.User != nil || uri.RawQuery != "" || uri.ForceQuery || uri.Fragment != "" {
			return "", fmt.Errorf("only local file URLs are supported")
		}
		path = uri.Path
	}
	if !utf8.ValidString(path) || !filepath.IsAbs(path) || len(path) > MaxPathBytes {
		return "", fmt.Errorf("use an absolute path or local file URL")
	}
	for _, r := range path {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("paths containing control characters are unsupported")
		}
	}
	return filepath.Clean(path), nil
}
func metadata(path string) (Entry, error) {
	info, err := os.Stat(path)
	if err != nil || (!info.Mode().IsRegular() && !info.IsDir()) {
		return Entry{}, fmt.Errorf("file or folder is unavailable or unsupported")
	}
	size := info.Size()
	if info.IsDir() {
		size = 0
	}
	return Entry{Path: path, Name: filepath.Base(path), Directory: info.IsDir(), Available: true, Size: size}, nil
}
func (m *Module) snapshotLocked() Snapshot {
	for i, old := range m.entries {
		entry, err := metadata(old.Path)
		if err != nil {
			entry = old
			entry.Available = false
			entry.Size = 0
		}
		entry.ID = old.ID
		if entry != old {
			m.entries[i] = entry
			m.revision++
		}
	}
	return Snapshot{Revision: m.revision, Entries: append([]Entry{}, m.entries...)}
}
func (m *Module) Snapshot() Snapshot { m.mu.Lock(); defer m.mu.Unlock(); return m.snapshotLocked() }
func (m *Module) Add(paths []string) (Snapshot, error) {
	if len(paths) == 0 || len(paths) > MaxEntries {
		return Snapshot{}, fmt.Errorf("add between 1 and 50 local paths")
	}
	// Validate the entire batch before changing shelf state.
	batch := make([]Entry, 0, len(paths))
	seen := make(map[string]bool)
	for _, value := range paths {
		path, err := localPath(value)
		if err != nil {
			return Snapshot{}, err
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		entry, err := metadata(path)
		if err != nil {
			return Snapshot{}, err
		}
		batch = append(batch, entry)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	remaining := make([]Entry, 0, len(m.entries))
	ids := make(map[string]string)
	for _, entry := range m.entries {
		ids[entry.Path] = entry.ID
		if !seen[entry.Path] {
			remaining = append(remaining, entry)
		}
	}
	total := 0
	for _, entry := range append(append([]Entry{}, batch...), remaining...) {
		total += len(entry.Path)
	}
	if len(batch)+len(remaining) > MaxEntries || total > MaxTotalPathBytes {
		return Snapshot{}, fmt.Errorf("shelf limit reached (50 items or 32 KiB of paths); remove an item first")
	}
	for i := range batch {
		batch[i].ID = ids[batch[i].Path]
		if batch[i].ID == "" {
			m.sequence++
			batch[i].ID = strconv.FormatUint(m.sequence, 10)
		}
	}
	m.entries = append(batch, remaining...)
	m.revision++
	return m.snapshotLocked(), nil
}
func (m *Module) Remove(id string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, entry := range m.entries {
		if entry.ID == id {
			copy(m.entries[i:], m.entries[i+1:])
			m.entries[len(m.entries)-1] = Entry{}
			m.entries = m.entries[:len(m.entries)-1]
			m.revision++
			return m.snapshotLocked(), nil
		}
	}
	return Snapshot{}, fmt.Errorf("shelf item is no longer available")
}
func (m *Module) Clear() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = []Entry{}
	m.revision++
	return m.snapshotLocked()
}
func (m *Module) item(id string) (Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range m.entries {
		if entry.ID == id {
			fresh, err := metadata(entry.Path)
			fresh.ID = id
			return fresh, err
		}
	}
	return Entry{}, fmt.Errorf("shelf item is no longer available")
}
func (m *Module) CopyPath(ctx context.Context, id string) (Snapshot, error) {
	entry, err := m.item(id)
	if err != nil {
		return Snapshot{}, err
	}
	if err := m.driver.CopyPath(ctx, entry.Path); err != nil {
		return Snapshot{}, err
	}
	return m.Snapshot(), nil
}
func (m *Module) OpenFolder(ctx context.Context, id string) (Snapshot, error) {
	entry, err := m.item(id)
	if err != nil {
		return Snapshot{}, err
	}
	dir := entry.Path
	if !entry.Directory {
		dir = filepath.Dir(dir)
	}
	if err := m.driver.OpenFolder(ctx, dir); err != nil {
		return Snapshot{}, err
	}
	return m.Snapshot(), nil
}
