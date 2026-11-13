// Package windows provides explicit switching between existing Hyprland windows.
// It launches no applications and stores no titles outside the current session.
package windows

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"omnibar/internal/modules"
)

const Name = "window_switcher"
const Capability = "window.manage"
const PreviewCapability = "screen.capture"
const MaxWindows = 100

var addressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{1,16}$`)

func ValidAddress(address string) bool { return addressPattern.MatchString(address) }

type Window struct {
	Address   string `json:"address"`
	Title     string `json:"title"`
	Class     string `json:"class"`
	Workspace string `json:"workspace"`
	Monitor   int    `json:"monitor"`
	Active    bool   `json:"active"`
}
type Snapshot struct {
	Revision  uint64   `json:"revision"`
	Truncated bool     `json:"truncated"`
	Windows   []Window `json:"windows"`
}
type Driver interface {
	Read(context.Context) ([]Window, string, error)
	Focus(context.Context, string) error
}
type Module struct {
	mu          sync.Mutex
	driver      Driver
	revision    uint64
	last        Snapshot
	previewGate chan struct{}
}

func New(driver Driver) *Module {
	if driver == nil {
		driver = Hyprland{}
	}
	return &Module{driver: driver, revision: 1, previewGate: make(chan struct{}, 1)}
}
func (m *Module) Info() modules.ModuleInfo {
	return modules.ModuleInfo{Name: Name, Description: "Switch between running Hyprland windows", Version: "0.1.0", Enabled: true, Capabilities: []string{Capability, PreviewCapability}}
}
func (m *Module) Initialize(context.Context) error { return nil }
func (m *Module) Start(context.Context) error      { return nil }
func (m *Module) Stop(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.last = Snapshot{}
	m.revision++
	return nil
}
func (m *Module) Dispose(ctx context.Context) error { return m.Stop(ctx) }
func label(text string, limit int) string {
	text = strings.ToValidUTF8(text, "�")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	if len(text) > limit {
		text = text[:limit]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	return strings.TrimSpace(text)
}
func (m *Module) snapshotLocked(ctx context.Context) (Snapshot, error) {
	entries, active, err := m.driver.Read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	result := make([]Window, 0, len(entries))
	seen := make(map[string]bool)
	for _, entry := range entries {
		if !ValidAddress(entry.Address) || seen[entry.Address] {
			continue
		}
		seen[entry.Address] = true
		entry.Title = label(entry.Title, 512)
		if entry.Title == "" {
			entry.Title = "Untitled window"
		}
		entry.Class = label(entry.Class, 128)
		if entry.Class == "" {
			entry.Class = "Unknown app"
		}
		entry.Workspace = label(entry.Workspace, 64)
		entry.Active = entry.Address == active
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if strings.ToLower(a.Class) != strings.ToLower(b.Class) {
			return strings.ToLower(a.Class) < strings.ToLower(b.Class)
		}
		if strings.ToLower(a.Title) != strings.ToLower(b.Title) {
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		}
		return a.Address < b.Address
	})
	truncated := len(result) > MaxWindows
	if truncated {
		result = result[:MaxWindows]
	}
	if !slices.Equal(result, m.last.Windows) || truncated != m.last.Truncated {
		m.revision++
	}
	m.last = Snapshot{Revision: m.revision, Truncated: truncated, Windows: result}
	return Snapshot{Revision: m.revision, Truncated: truncated, Windows: append([]Window{}, result...)}, nil
}
func (m *Module) Snapshot(ctx context.Context) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked(ctx)
}
func (m *Module) Focus(ctx context.Context, address string) (Snapshot, error) {
	if !ValidAddress(address) {
		return Snapshot{}, fmt.Errorf("invalid window address")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	snap, err := m.snapshotLocked(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if !slices.ContainsFunc(snap.Windows, func(w Window) bool { return w.Address == address }) {
		return Snapshot{}, fmt.Errorf("window is no longer available; refresh and try again")
	}
	if err := m.driver.Focus(ctx, address); err != nil {
		return Snapshot{}, err
	}
	confirmed, err := m.snapshotLocked(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("focus requested but could not verify the active window; refresh")
	}
	if !slices.ContainsFunc(confirmed.Windows, func(w Window) bool { return w.Address == address && w.Active }) {
		return Snapshot{}, fmt.Errorf("window focus was not confirmed; refresh and try again")
	}
	return confirmed, nil
}
