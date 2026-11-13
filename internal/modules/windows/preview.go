package windows

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const MaxPreviewBytes = 128 * 1024
const maxPreviewPixels = 4 * 1024 * 1024

type Preview struct {
	Address string `json:"address"`
	DataURL string `json:"dataURL"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}
type PreviewDriver interface {
	Capture(context.Context, string) ([]byte, error)
}

func (m *Module) Preview(ctx context.Context, address string) (Preview, error) {
	if !ValidAddress(address) {
		return Preview{}, fmt.Errorf("invalid window address")
	}
	select {
	case m.previewGate <- struct{}{}:
		defer func() { <-m.previewGate }()
	default:
		return Preview{}, fmt.Errorf("a preview is already being captured; try again")
	}
	snap, err := m.Snapshot(ctx)
	if err != nil {
		return Preview{}, err
	}
	if !slices.ContainsFunc(snap.Windows, func(w Window) bool { return w.Address == address }) {
		return Preview{}, fmt.Errorf("window is no longer available; refresh and try again")
	}
	driver, ok := m.driver.(PreviewDriver)
	if !ok {
		return Preview{}, fmt.Errorf("window previews are unavailable")
	}
	data, err := driver.Capture(ctx, address)
	if err != nil {
		return Preview{}, err
	}
	return thumbnail(ctx, address, data)
}

func thumbnail(ctx context.Context, address string, data []byte) (Preview, error) {
	fail := func() (Preview, error) { return Preview{}, fmt.Errorf("could not create a bounded window preview") }
	if len(data) > maxReplyBytes {
		return fail()
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 || config.Width*config.Height > maxPreviewPixels {
		return fail()
	}
	if ctx.Err() != nil {
		return fail()
	}
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fail()
	}
	scale := math.Min(1, math.Min(320/float64(config.Width), 200/float64(config.Height)))
	w, h := max(1, int(float64(config.Width)*scale)), max(1, int(float64(config.Height)*scale))
	result := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		if ctx.Err() != nil {
			return fail()
		}
		for x := 0; x < w; x++ {
			result.Set(x, y, source.At(x*config.Width/w, y*config.Height/h))
		}
	}
	var out bytes.Buffer
	if jpeg.Encode(&out, result, &jpeg.Options{Quality: 75}) != nil || out.Len() > MaxPreviewBytes {
		return fail()
	}
	return Preview{Address: address, DataURL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(out.Bytes()), Width: w, Height: h}, nil
}

var stableIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)

func stableTarget(data []byte, address string) (string, error) {
	var clients []client
	if json.Unmarshal(data, &clients) != nil {
		return "", fmt.Errorf("could not read the preview target")
	}
	for _, c := range clients {
		if c.Address == address && c.eligible() && stableIDPattern.MatchString(c.StableID) {
			return c.StableID, nil
		}
	}
	return "", fmt.Errorf("exact window preview is unavailable; refresh and try again")
}

// Resolve the Wayland socket from the same explicit compositor signature as
// hyprctl. Never capture against a stale inherited display or WAYLAND_SOCKET FD.
func captureDisplay(ctx context.Context) (string, error) {
	data, err := command(ctx, "-j", "instances")
	if err != nil {
		return "", err
	}
	var instances []struct {
		Instance string `json:"instance"`
		Socket   string `json:"wl_socket"`
	}
	if json.Unmarshal(data, &instances) != nil {
		return "", fmt.Errorf("preview compositor is unavailable")
	}
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	for _, instance := range instances {
		if instance.Instance == os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") && filepath.IsAbs(runtime) && instance.Socket != "" && instance.Socket != "." && instance.Socket != ".." && filepath.Base(instance.Socket) == instance.Socket {
			return filepath.Join(runtime, instance.Socket), nil
		}
	}
	return "", fmt.Errorf("preview compositor is unavailable")
}

func (Hyprland) Capture(ctx context.Context, address string) ([]byte, error) {
	if !ValidAddress(address) {
		return nil, fmt.Errorf("invalid window address")
	}
	display, err := captureDisplay(ctx)
	if err != nil {
		return nil, err
	}
	data, err := command(ctx, "-j", "clients")
	if err != nil {
		return nil, err
	}
	id, err := stableTarget(data, address)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "grim", "-T", id, "-s", "0.25", "-t", "png", "-")
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "WAYLAND_DISPLAY=") && !strings.HasPrefix(value, "WAYLAND_SOCKET=") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "WAYLAND_DISPLAY="+display)
	out := &boundedOutput{}
	cmd.Stdout = out
	// stderr stays disconnected; screenshot data and compositor diagnostics are
	// never written to logs or disk. There is no output/region capture fallback.
	if cmd.Run() != nil || out.overflow {
		return nil, fmt.Errorf("window preview unavailable; check grim and compositor capture support")
	}
	data, err = command(ctx, "-j", "clients")
	if err != nil {
		return nil, err
	}
	confirmed, err := stableTarget(data, address)
	if err != nil || confirmed != id {
		return nil, fmt.Errorf("window changed during capture; refresh and try again")
	}
	return out.Bytes(), nil
}
