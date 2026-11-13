package windows

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxLayouts = 8
const MaxLayoutWindows = 16

type Placement struct {
	Class     string   `json:"class"`
	Title     string   `json:"title"`
	Floating  bool     `json:"floating"`
	Maximized bool     `json:"maximized"`
	Geometry  Geometry `json:"geometry"`
}
type Layout struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Workspace int         `json:"workspace"`
	Topology  string      `json:"topology"`
	Windows   []Placement `json:"windows"`
}
type LayoutSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Workspace int    `json:"workspace"`
	Count     int    `json:"count"`
}
type LayoutDriver interface {
	ControlDriver
	Topology(context.Context) (string, error)
}

func ValidLayoutName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name && len(name) <= 64 && utf8.ValidString(name) && strings.IndexFunc(name, unicode.IsControl) < 0
}
func ValidLayoutID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16 && id == strings.ToLower(id)
}
func validateLayouts(layouts []Layout) error {
	if len(layouts) > MaxLayouts {
		return fmt.Errorf("layout limit reached (8)")
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for _, l := range layouts {
		if !ValidLayoutID(l.ID) || !ValidLayoutName(l.Name) || ids[l.ID] || names[strings.ToLower(l.Name)] || l.Workspace < 1 || l.Workspace > 9999 || len(l.Topology) != 64 || len(l.Windows) < 1 || len(l.Windows) > MaxLayoutWindows {
			return fmt.Errorf("invalid saved layout")
		}
		topologyBytes, err := hex.DecodeString(l.Topology)
		if err != nil || len(topologyBytes) != 32 {
			return fmt.Errorf("invalid monitor fingerprint")
		}
		ids[l.ID] = true
		names[strings.ToLower(l.Name)] = true
		seen := map[string]bool{}
		for _, w := range l.Windows {
			key := w.Class + "\x00" + w.Title
			if w.Class == "" || len(w.Class) > 128 || len(w.Title) > 512 || textLabelInvalid(w.Class) || textLabelInvalid(w.Title) || seen[key] || (w.Floating && !w.Maximized && !w.Geometry.Valid()) {
				return fmt.Errorf("invalid or ambiguous layout windows")
			}
			seen[key] = true
		}
	}
	return nil
}
func textLabelInvalid(s string) bool {
	return !utf8.ValidString(s) || strings.IndexFunc(s, unicode.IsControl) >= 0
}
func (m *Module) layoutSummaries() []LayoutSummary {
	result := make([]LayoutSummary, 0, len(m.layouts))
	for _, l := range m.layouts {
		result = append(result, LayoutSummary{l.ID, l.Name, l.Workspace, len(l.Windows)})
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name) })
	return result
}
func (m *Module) SaveLayout(ctx context.Context, name string, workspace int) (Snapshot, error) {
	if !ValidLayoutName(name) || workspace < 1 || workspace > 9999 {
		return Snapshot{}, fmt.Errorf("invalid layout name or workspace ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.layoutErr != "" {
		return Snapshot{}, fmt.Errorf("saved layouts could not be loaded; original file preserved")
	}
	if m.layoutStore == nil {
		return Snapshot{}, fmt.Errorf("layout storage is unavailable")
	}
	driver, ok := m.driver.(LayoutDriver)
	if !ok {
		return Snapshot{}, fmt.Errorf("saved layouts are unavailable")
	}
	snap, err := m.snapshotLocked(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if snap.Truncated {
		return Snapshot{}, fmt.Errorf("cannot save a truncated window list")
	}
	topology, err := driver.Topology(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return Snapshot{}, fmt.Errorf("could not create layout ID")
	}
	l := Layout{ID: hex.EncodeToString(id), Name: name, Workspace: workspace, Topology: topology}
	for _, w := range snap.Windows {
		if w.WorkspaceID != workspace {
			continue
		}
		if w.Restricted || w.Fullscreen >= 2 || w.Class == "Unknown app" {
			return Snapshot{}, fmt.Errorf("restore fullscreen, grouped, or pinned windows before saving")
		}
		l.Windows = append(l.Windows, Placement{w.Class, w.Title, w.Floating, w.Fullscreen == 1, w.Geometry})
	}
	if len(l.Windows) == 0 || len(l.Windows) > MaxLayoutWindows {
		return Snapshot{}, fmt.Errorf("save a workspace with 1–16 windows")
	}
	next := append([]Layout{}, m.layouts...)
	index := -1
	for i, old := range next {
		if strings.EqualFold(old.Name, name) {
			index = i
			l.ID = old.ID
			break
		}
	}
	if index >= 0 {
		next[index] = l
	} else {
		next = append(next, l)
	}
	if err := validateLayouts(next); err != nil {
		return Snapshot{}, err
	}
	if err := m.layoutStore.Save(next); err != nil {
		return Snapshot{}, err
	}
	m.layouts = next
	return m.layoutUpdatedSnapshot(snap), nil
}
func (m *Module) DeleteLayout(ctx context.Context, id string) (Snapshot, error) {
	if !ValidLayoutID(id) {
		return Snapshot{}, fmt.Errorf("invalid layout ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.layoutErr != "" || m.layoutStore == nil {
		return Snapshot{}, fmt.Errorf("layout storage is unavailable; original file preserved")
	}
	snap, err := m.snapshotLocked(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	next := []Layout{}
	found := false
	for _, l := range m.layouts {
		if l.ID == id {
			found = true
		} else {
			next = append(next, l)
		}
	}
	if !found {
		return Snapshot{}, fmt.Errorf("saved layout is no longer available")
	}
	if err := m.layoutStore.Save(next); err != nil {
		return Snapshot{}, err
	}
	m.layouts = next
	return m.layoutUpdatedSnapshot(snap), nil
}

type restoreTarget struct {
	window    Window
	placement Placement
}

func resolveLayout(l Layout, windows []Window) ([]restoreTarget, error) {
	result := []restoreTarget{}
	used := map[string]bool{}
	for _, p := range l.Windows {
		classMatches, titleMatches := []Window{}, []Window{}
		for _, w := range windows {
			if w.Class == p.Class {
				classMatches = append(classMatches, w)
				if w.Title == p.Title {
					titleMatches = append(titleMatches, w)
				}
			}
		}
		var w Window
		if len(titleMatches) == 1 {
			w = titleMatches[0]
		} else if len(classMatches) == 1 {
			w = classMatches[0]
		} else {
			return nil, fmt.Errorf("layout windows are missing or ambiguous; open the matching apps first")
		}
		if used[w.Address] || w.Restricted || w.Fullscreen >= 2 || w.Identity == "" {
			return nil, fmt.Errorf("layout targets are ambiguous or cannot be controlled")
		}
		used[w.Address] = true
		result = append(result, restoreTarget{w, p})
	}
	return result, nil
}
func (m *Module) RestoreLayout(ctx context.Context, id string) (Snapshot, error) {
	if !ValidLayoutID(id) {
		return Snapshot{}, fmt.Errorf("invalid layout ID")
	}
	select {
	case m.layoutGate <- struct{}{}:
		defer func() { <-m.layoutGate }()
	default:
		return Snapshot{}, fmt.Errorf("a layout is already restoring")
	}
	m.mu.Lock()
	var saved *Layout
	for i := range m.layouts {
		if m.layouts[i].ID == id {
			copy := m.layouts[i]
			copy.Windows = append([]Placement{}, copy.Windows...)
			saved = &copy
			break
		}
	}
	m.mu.Unlock()
	if saved == nil {
		return Snapshot{}, fmt.Errorf("saved layout is no longer available")
	}
	driver, ok := m.driver.(LayoutDriver)
	if !ok {
		return Snapshot{}, fmt.Errorf("saved layouts are unavailable")
	}
	topology, err := driver.Topology(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if topology != saved.Topology {
		return Snapshot{}, fmt.Errorf("monitor arrangement changed; save this layout again")
	}
	snap, err := m.Snapshot(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if snap.Truncated {
		return Snapshot{}, fmt.Errorf("cannot restore from a truncated window list")
	}
	pairs, err := resolveLayout(*saved, snap.Windows)
	if err != nil {
		return Snapshot{}, err
	}
	for _, pair := range pairs {
		current, e := driver.Topology(ctx)
		if e != nil || current != saved.Topology {
			return Snapshot{}, fmt.Errorf("monitor arrangement changed; restore stopped and may be partial")
		}
		steps := []Change{{Action: "maximized", Enabled: false}, {Action: "workspace", Workspace: saved.Workspace}, {Action: "floating", Enabled: pair.placement.Floating}}
		if pair.placement.Floating && !pair.placement.Maximized {
			steps = append(steps, Change{Action: "geometry", Geometry: pair.placement.Geometry})
		}
		if pair.placement.Maximized {
			steps = append(steps, Change{Action: "maximized", Enabled: true})
		}
		for _, c := range steps {
			c.Identity = pair.window.Identity
			fresh, e := m.Snapshot(ctx)
			if e != nil {
				return Snapshot{}, fmt.Errorf("restore stopped; desktop may be partially changed")
			}
			w, e := target(fresh, pair.window.Address)
			if e != nil || w.Identity != pair.window.Identity {
				return Snapshot{}, fmt.Errorf("layout target changed; restore stopped and may be partial")
			}
			if _, e = m.Change(ctx, w.Address, c); e != nil {
				return Snapshot{}, fmt.Errorf("restore stopped; desktop may be partially changed: %s", e.Error())
			}
		}
	}
	current, e := driver.Topology(ctx)
	if e != nil || current != saved.Topology {
		return Snapshot{}, fmt.Errorf("monitor arrangement changed; restore may be partial")
	}
	snap, err = m.Snapshot(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	for _, pair := range pairs {
		w, e := target(snap, pair.window.Address)
		p := pair.placement
		if e != nil || w.Identity != pair.window.Identity || w.WorkspaceID != saved.Workspace || w.Floating != p.Floating || (w.Fullscreen == 1) != p.Maximized || (p.Floating && !p.Maximized && w.Geometry != p.Geometry) {
			return Snapshot{}, fmt.Errorf("compositor adjusted layout state; restore may be partial")
		}
	}
	return snap, nil
}
func (Hyprland) Topology(ctx context.Context) (string, error) {
	data, err := command(ctx, "-j", "monitors")
	if err != nil {
		return "", err
	}
	var monitors []struct {
		Name                           string `json:"name"`
		X, Y, Width, Height, Transform int
		Scale                          float64
	}
	if json.Unmarshal(data, &monitors) != nil || len(monitors) == 0 {
		return "", fmt.Errorf("monitor arrangement is unavailable")
	}
	for _, monitor := range monitors {
		if monitor.Name == "" || monitor.Width <= 0 || monitor.Height <= 0 || monitor.Scale <= 0 {
			return "", fmt.Errorf("monitor arrangement is unavailable")
		}
	}
	sort.Slice(monitors, func(i, j int) bool { return monitors[i].Name < monitors[j].Name })
	normalized, _ := json.Marshal(monitors)
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:]), nil
}

func (m *Module) layoutUpdatedSnapshot(s Snapshot) Snapshot {
	m.revision++
	s.Revision = m.revision
	s.Layouts = m.layoutSummaries()
	s.LayoutError = m.layoutErr
	m.last = s
	m.last.Windows = append([]Window{}, s.Windows...)
	m.last.Layouts = append([]LayoutSummary{}, s.Layouts...)
	return s
}
