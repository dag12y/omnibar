package windows

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func previewPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{40, 90, 130, 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func TestPreviewThumbnailBoundsAndPixels(t *testing.T) {
	p, err := thumbnail(context.Background(), "0x1", previewPNG(t, 1024, 600))
	if err != nil {
		t.Fatal(err)
	}
	if p.Width != 320 || p.Height != 187 || p.Address != "0x1" {
		t.Fatalf("unexpected thumbnail: %dx%d", p.Width, p.Height)
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(p.DataURL, "data:image/jpeg;base64,"))
	if err != nil || len(data) > MaxPreviewBytes {
		t.Fatal("invalid bounded JPEG", err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	c := color.RGBAModel.Convert(img.At(10, 10)).(color.RGBA)
	if c.R < 35 || c.R > 45 || c.G < 85 || c.G > 95 || c.B < 125 || c.B > 135 {
		t.Fatal("thumbnail lost source content", c)
	}
	frame, _ := json.Marshal(p)
	if len(frame) > 200000 {
		t.Fatal("preview escaped IPC bounds")
	}
	portrait, err := thumbnail(context.Background(), "0x1", previewPNG(t, 20, 40))
	if err != nil || portrait.Width != 20 || portrait.Height != 40 {
		t.Fatal("small image was distorted")
	}
	for _, data := range [][]byte{nil, []byte("not a PNG"), make([]byte, maxReplyBytes+1), previewPNG(t, 4097, 1)} {
		if _, err := thumbnail(context.Background(), "0x1", data); err == nil {
			t.Fatal("invalid/oversized image accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := thumbnail(ctx, "0x1", previewPNG(t, 2, 2)); err == nil {
		t.Fatal("cancelled thumbnail accepted")
	}
}

type previewDriver struct {
	fakeDriver
	data    []byte
	started chan struct{}
	release chan struct{}
}

func (f *previewDriver) Capture(ctx context.Context, _ string) ([]byte, error) {
	if f.started != nil {
		close(f.started)
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.data, nil
}
func TestPreviewLiveTargetGateAndFocusRemainsAvailable(t *testing.T) {
	f := &previewDriver{fakeDriver: fakeDriver{entries: []Window{{Address: "0x1", Title: "Fixture"}}}, data: previewPNG(t, 4, 4), started: make(chan struct{}), release: make(chan struct{})}
	m := New(f)
	if _, err := m.Preview(context.Background(), "0x2"); err == nil {
		t.Fatal("stale target accepted")
	}
	done := make(chan error, 1)
	go func() { _, err := m.Preview(context.Background(), "0x1"); done <- err }()
	<-f.started
	if _, err := m.Preview(context.Background(), "0x1"); err == nil {
		t.Fatal("parallel capture accepted")
	}
	if _, err := m.Focus(context.Background(), "0x1"); err != nil {
		t.Fatal("capture blocked focus", err)
	}
	close(f.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := m.Preview(context.Background(), "invalid"); err == nil {
		t.Fatal("invalid target accepted")
	}
}

func TestExactCaptureCommandSessionIdentityAndFailure(t *testing.T) {
	dir := t.TempDir()
	runtime := t.TempDir()
	clients := filepath.Join(dir, "clients.json")
	instances := filepath.Join(dir, "instances.json")
	source := filepath.Join(dir, "source.png")
	log := filepath.Join(dir, "capture.log")
	const live = `[{"address":"0x1","stableId":"12345","mapped":true,"workspace":{"name":"1"}}]`
	for path, data := range map[string][]byte{clients: []byte(live), instances: []byte(`[{"instance":"fixture","wl_socket":"wayland-test"}]`), source: previewPNG(t, 3, 3)} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	tool := `#!/bin/sh
case "$2" in
 clients) exec /bin/cat "$PREVIEW_CLIENTS" ;;
 instances) exec /bin/cat "$PREVIEW_INSTANCES" ;;
esac
exit 1
`
	grim := `#!/bin/sh
printf '%s\n' "$WAYLAND_DISPLAY" "${WAYLAND_SOCKET-unset}" "$@" > "$PREVIEW_LOG"
case "$PREVIEW_MODE" in
 fail) echo 'private title must not escape' >&2; exit 1 ;;
 timeout) exec /bin/sleep 5 ;;
 overflow) exec /usr/bin/head -c 2097153 /dev/zero ;;
 changed) printf '%s' '[]' > "$PREVIEW_CLIENTS" ;;
 reused) printf '%s' '[{"address":"0x1","stableId":"54321","mapped":true}]' > "$PREVIEW_CLIENTS" ;;
esac
exec /bin/cat "$PREVIEW_SOURCE"
`
	for name, data := range map[string]string{"hyprctl": tool, "grim": grim} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for key, value := range map[string]string{"PATH": dir, "XDG_RUNTIME_DIR": runtime, "HYPRLAND_INSTANCE_SIGNATURE": "fixture", "WAYLAND_DISPLAY": "wrong-display", "WAYLAND_SOCKET": "99", "PREVIEW_CLIENTS": clients, "PREVIEW_INSTANCES": instances, "PREVIEW_SOURCE": source, "PREVIEW_LOG": log} {
		t.Setenv(key, value)
	}
	data, err := (Hyprland{}).Capture(context.Background(), "0x1")
	if err != nil || !bytes.Equal(data, previewPNG(t, 3, 3)) {
		t.Fatal("exact capture failed", err)
	}
	args, _ := os.ReadFile(log)
	want := filepath.Join(runtime, "wayland-test") + "\nunset\n-T\n12345\n-s\n0.25\n-t\npng\n-\n"
	if string(args) != want {
		t.Fatalf("capture escaped target/session selection: %q", args)
	}
	for _, mode := range []string{"fail", "timeout", "overflow", "changed", "reused"} {
		t.Run(mode, func(t *testing.T) {
			os.WriteFile(clients, []byte(live), 0600)
			t.Setenv("PREVIEW_MODE", mode)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if _, err := (Hyprland{}).Capture(ctx, "0x1"); err == nil || strings.Contains(err.Error(), "private title") {
				t.Fatal("capture failure hidden or output leaked", err)
			}
		})
	}
	os.WriteFile(instances, []byte(`[{"instance":"other","wl_socket":"wayland-test"}]`), 0600)
	if _, err := (Hyprland{}).Capture(context.Background(), "0x1"); err == nil {
		t.Fatal("wrong compositor selected")
	}
	for _, data := range []string{`[{"address":"0x1","stableId":"123;bad","mapped":true}]`, `[{"address":"0x1","mapped":true}]`, `[{"address":"0x1","stableId":"123","mapped":true,"hidden":true}]`} {
		if _, err := stableTarget([]byte(data), "0x1"); err == nil {
			t.Fatal("invalid/hidden capture target accepted")
		}
	}
}
