package windows

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const maxReplyBytes = 2 * 1024 * 1024

// A bounded writer prevents compositor output from growing backend memory.
type boundedOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	n := len(data)
	remaining := maxReplyBytes - b.Len()
	if remaining < len(data) {
		b.overflow = true
		data = data[:remaining]
	}
	_, _ = b.Buffer.Write(data)
	return n, nil
}

type Hyprland struct{}

func command(ctx context.Context, args ...string) ([]byte, error) {
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		return nil, fmt.Errorf("Hyprland session is unavailable; restart Omnibar from your desktop session")
	}
	cmd := exec.CommandContext(ctx, "hyprctl", args...)
	out := &boundedOutput{}
	cmd.Stdout = out
	// Never propagate compositor output to errors: it can contain private titles.
	if err := cmd.Run(); err != nil || out.overflow {
		return nil, fmt.Errorf("Hyprland unavailable; check hyprctl and the compositor session")
	}
	return out.Bytes(), nil
}

type client struct {
	Address      string `json:"address"`
	Title        string `json:"title"`
	Class        string `json:"class"`
	InitialClass string `json:"initialClass"`
	Workspace    struct {
		Name string `json:"name"`
	} `json:"workspace"`
	Monitor      int   `json:"monitor"`
	Mapped       bool  `json:"mapped"`
	Hidden       bool  `json:"hidden"`
	AcceptsInput *bool `json:"acceptsInput"`
}

func parseClients(data []byte) ([]Window, error) {
	var clients []client
	if json.Unmarshal(data, &clients) != nil || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, fmt.Errorf("could not read Hyprland windows")
	}
	result := make([]Window, 0, len(clients))
	for _, c := range clients {
		if !c.Mapped || c.Hidden || (c.AcceptsInput != nil && !*c.AcceptsInput) || (c.Workspace.Name == "special" || strings.HasPrefix(c.Workspace.Name, "special:")) {
			continue
		}
		if c.Class == "" {
			c.Class = c.InitialClass
		}
		result = append(result, Window{Address: c.Address, Title: c.Title, Class: c.Class, Workspace: c.Workspace.Name, Monitor: c.Monitor})
	}
	return result, nil
}
func (Hyprland) Read(ctx context.Context) ([]Window, string, error) {
	data, err := command(ctx, "-j", "clients")
	if err != nil {
		return nil, "", err
	}
	windows, err := parseClients(data)
	if err != nil {
		return nil, "", err
	}
	data, err = command(ctx, "-j", "activewindow")
	if err != nil {
		return nil, "", err
	}
	var active struct {
		Address string `json:"address"`
	}
	if json.Unmarshal(data, &active) != nil || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, "", fmt.Errorf("could not read the active Hyprland window")
	}
	return windows, active.Address, nil
}
func (Hyprland) Focus(ctx context.Context, address string) error {
	if !ValidAddress(address) {
		return fmt.Errorf("invalid window address")
	}
	out, err := command(ctx, "dispatch", "focuswindow", "address:"+address)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != "ok" {
		return fmt.Errorf("Hyprland did not accept the focus request")
	}
	return nil
}
