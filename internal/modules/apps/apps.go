// Package apps discovers installed desktop entries and delegates explicit
// launches to GIO, which implements desktop Exec/terminal/D-Bus semantics.
package apps

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"omnibar/internal/modules"
)

const Name = "application_launcher"
const Capability = "process.manage"
const MaxApps = 200

type App struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Keywords    string `json:"keywords"`
}
type Snapshot struct {
	Revision  uint64 `json:"revision"`
	Truncated bool   `json:"truncated"`
	Apps      []App  `json:"apps"`
}
type Driver interface {
	Read(context.Context) ([]App, bool, error)
	Launch(context.Context, string) error
}
type Module struct {
	mu     sync.Mutex
	driver Driver
	last   Snapshot
}

func New(driver Driver) *Module {
	if driver == nil {
		driver = Desktop{}
	}
	return &Module{driver: driver}
}
func (m *Module) Info() modules.ModuleInfo {
	return modules.ModuleInfo{Name: Name, Description: "Launch installed desktop applications", Version: "0.1.0", Enabled: true, Capabilities: []string{Capability}}
}
func (m *Module) Initialize(context.Context) error { return nil }
func (m *Module) Start(context.Context) error      { return nil }
func (m *Module) Stop(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.last = Snapshot{Revision: m.last.Revision + 1}
	return nil
}
func (m *Module) Dispose(ctx context.Context) error { return m.Stop(ctx) }
func (m *Module) snapshot(ctx context.Context) (Snapshot, error) {
	entries, truncated, err := m.driver.Read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	clean := make([]App, 0, len(entries))
	seen := make(map[string]bool)
	for _, a := range entries {
		if !ValidID(a.ID) || seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		a.Name = textLabel(a.Name, 128)
		a.Description = textLabel(a.Description, 128)
		a.Keywords = textLabel(a.Keywords, 128)
		if a.Name != "" {
			clean = append(clean, a)
		}
	}
	entries = clean
	sort.Slice(entries, func(i, j int) bool {
		if strings.ToLower(entries[i].Name) != strings.ToLower(entries[j].Name) {
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		}
		return entries[i].ID < entries[j].ID
	})
	if len(entries) > MaxApps {
		entries = entries[:MaxApps]
		truncated = true
	}
	revision := m.last.Revision
	if !slices.Equal(entries, m.last.Apps) || truncated != m.last.Truncated || revision == 0 {
		revision++
	}
	m.last = Snapshot{Revision: revision, Truncated: truncated, Apps: append([]App{}, entries...)}
	return Snapshot{Revision: revision, Truncated: truncated, Apps: append([]App{}, entries...)}, nil
}
func (m *Module) Snapshot(ctx context.Context) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshot(ctx)
}
func (m *Module) Launch(ctx context.Context, id string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap, err := m.snapshot(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if !slices.ContainsFunc(snap.Apps, func(a App) bool { return a.ID == id }) {
		return Snapshot{}, fmt.Errorf("application is no longer available; refresh and try again")
	}
	if err := m.driver.Launch(ctx, id); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}
