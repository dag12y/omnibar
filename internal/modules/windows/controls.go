package windows

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Geometry struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (g Geometry) Valid() bool {
	return g.X >= -100000 && g.X <= 100000 && g.Y >= -100000 && g.Y <= 100000 && g.Width >= 16 && g.Width <= 16384 && g.Height >= 16 && g.Height <= 16384
}

type Change struct {
	Identity  string
	Action    string
	Workspace int
	Enabled   bool
	Geometry  Geometry
}
type ControlDriver interface {
	Apply(context.Context, string, Change) error
}

func (c Change) Valid() bool {
	switch c.Action {
	case "workspace":
		return c.Workspace >= 1 && c.Workspace <= 9999
	case "floating", "maximized":
		return true
	case "geometry":
		return c.Geometry.Valid()
	}
	return false
}
func matches(w Window, c Change) bool {
	switch c.Action {
	case "workspace":
		return w.WorkspaceID == c.Workspace
	case "floating":
		return w.Floating == c.Enabled
	case "maximized":
		return (w.Fullscreen == 1) == c.Enabled && w.Fullscreen < 2
	case "geometry":
		return w.Geometry == c.Geometry
	}
	return false
}
func target(s Snapshot, address string) (Window, error) {
	for _, w := range s.Windows {
		if w.Address == address {
			return w, nil
		}
	}
	return Window{}, fmt.Errorf("window is no longer available; refresh and try again")
}
func (m *Module) Change(ctx context.Context, address string, c Change) (Snapshot, error) {
	if !ValidAddress(address) || !c.Valid() {
		return Snapshot{}, fmt.Errorf("invalid window control parameters")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.changeLocked(ctx, address, c)
}
func (m *Module) changeLocked(ctx context.Context, address string, c Change) (Snapshot, error) {
	snap, err := m.snapshotLocked(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	w, err := target(snap, address)
	if err != nil {
		return Snapshot{}, err
	}
	if c.Identity != "" && w.Identity != c.Identity {
		return Snapshot{}, fmt.Errorf("window identity changed; refresh")
	}
	if w.Restricted {
		return Snapshot{}, fmt.Errorf("grouped or pinned windows cannot be changed individually")
	}
	if matches(w, c) {
		return snap, nil
	}
	if w.Fullscreen >= 2 || ((c.Action == "floating" || c.Action == "geometry") && w.Fullscreen != 0) {
		return Snapshot{}, fmt.Errorf("restore the window from maximized/fullscreen mode first")
	}
	if c.Action == "geometry" && !w.Floating {
		return Snapshot{}, fmt.Errorf("window geometry requires floating mode")
	}
	driver, ok := m.driver.(ControlDriver)
	if !ok {
		return Snapshot{}, fmt.Errorf("window controls are unavailable")
	}
	if err := driver.Apply(ctx, address, c); err != nil {
		return Snapshot{}, err
	}
	for i := 0; i < 6; i++ {
		snap, err = m.snapshotLocked(ctx)
		if err != nil {
			return Snapshot{}, fmt.Errorf("change requested but state could not be verified; refresh")
		}
		w, err = target(snap, address)
		if err != nil {
			return Snapshot{}, err
		}
		if matches(w, c) {
			return snap, nil
		}
		select {
		case <-ctx.Done():
			return Snapshot{}, fmt.Errorf("window change was not confirmed; refresh")
		case <-time.After(40 * time.Millisecond):
		}
	}
	return Snapshot{}, fmt.Errorf("window change was not confirmed; refresh")
}
func dispatchExact(ctx context.Context, legacy []string, lua string) error {
	if len(legacy) > 0 {
		out, err := command(ctx, append([]string{"dispatch"}, legacy...)...)
		if err == nil && strings.TrimSpace(string(out)) == "ok" {
			return nil
		}
	}
	out, err := command(ctx, "dispatch", lua)
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		return fmt.Errorf("targeted window control unavailable; check Hyprland Lua support")
	}
	return nil
}
func (Hyprland) Apply(ctx context.Context, address string, c Change) error {
	if !ValidAddress(address) || !c.Valid() {
		return fmt.Errorf("invalid window control parameters")
	}
	selector := `"address:` + address + `"`
	switch c.Action {
	case "workspace":
		id := strconv.Itoa(c.Workspace)
		return dispatchExact(ctx, []string{"movetoworkspacesilent", id + ",address:" + address}, `hl.dsp.window.move({ window = `+selector+`, workspace = "`+id+`", follow = false })`)
	case "floating":
		verb, action := "settiled", "off"
		if c.Enabled {
			verb, action = "setfloating", "on"
		}
		return dispatchExact(ctx, []string{verb, "address:" + address}, `hl.dsp.window.float({ window = `+selector+`, action = "`+action+`" })`)
	case "maximized":
		action := "unset"
		if c.Enabled {
			action = "set"
		}
		// Legacy fullscreen dispatch acts on the focused window, so it must
		// never be used as a fallback for a selected background target.
		return dispatchExact(ctx, nil, `hl.dsp.window.fullscreen({ window = `+selector+`, mode = "maximized", action = "`+action+`" })`)
	case "geometry":
		g := c.Geometry
		if err := dispatchExact(ctx, []string{"resizewindowpixel", fmt.Sprintf("exact %d %d,address:%s", g.Width, g.Height, address)}, fmt.Sprintf(`hl.dsp.window.resize({ window = %s, x = %d, y = %d, relative = false })`, selector, g.Width, g.Height)); err != nil {
			return err
		}
		return dispatchExact(ctx, []string{"movewindowpixel", fmt.Sprintf("exact %d %d,address:%s", g.X, g.Y, address)}, fmt.Sprintf(`hl.dsp.window.move({ window = %s, x = %d, y = %d, relative = false })`, selector, g.X, g.Y))
	}
	return fmt.Errorf("unsupported window control")
}
