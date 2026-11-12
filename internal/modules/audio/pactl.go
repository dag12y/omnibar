package audio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

type commandFunc func(context.Context, ...string) ([]byte, error)
type Pactl struct{ command commandFunc }

func NewPactl() *Pactl { return &Pactl{command: runPactl} }
func pactlCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "pactl", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd
}
func runPactl(ctx context.Context, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := pactlCommand(ctx, args...)
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pactl: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return data, nil
}

type pulseVolume map[string]struct {
	Value uint32 `json:"value"`
}

func (v pulseVolume) percent() int {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, ch := range v {
		sum += float64(ch.Value)
	}
	return int(math.Round(sum / float64(len(v)) / 65536 * 100))
}

type pulseObject struct {
	Index       uint32            `json:"index"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Sink        uint32            `json:"sink"`
	Mute        bool              `json:"mute"`
	Corked      bool              `json:"corked"`
	Volume      pulseVolume       `json:"volume"`
	Properties  map[string]string `json:"properties"`
}

func (p *Pactl) Read(ctx context.Context) (Snapshot, error) {
	snap := Snapshot{Outputs: []Output{}, Streams: []Stream{}}
	read := func(target any, args ...string) error {
		data, err := p.command(ctx, append([]string{"--format=json"}, args...)...)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, target); err != nil {
			return fmt.Errorf("invalid pactl JSON: %w", err)
		}
		return nil
	}
	var info struct {
		Server  string `json:"server_name"`
		Default string `json:"default_sink_name"`
	}
	if err := read(&info, "info"); err != nil {
		return snap, err
	}
	var outputs, streams []pulseObject
	if err := read(&outputs, "list", "sinks"); err != nil {
		return snap, err
	}
	if err := read(&streams, "list", "sink-inputs"); err != nil {
		return snap, err
	}
	snap.Available, snap.Server, snap.DefaultOutput = true, info.Server, info.Default
	for _, o := range outputs {
		description := o.Description
		if description == "" {
			description = o.Name
		}
		snap.Outputs = append(snap.Outputs, Output{ID: strconv.FormatUint(uint64(o.Index), 10), Name: o.Name, Description: description, Volume: o.Volume.percent(), Muted: o.Mute})
	}
	for _, stream := range streams {
		props := stream.Properties
		name := props["application.name"]
		if name == "" {
			name = props["application.process.binary"]
		}
		if name == "" {
			name = props["media.name"]
		}
		if name == "" {
			name = "Audio stream"
		}
		id := strconv.FormatUint(uint64(stream.Index), 10)
		// The key detects most stale UI targets, including recreated streams with
		// recycled indexes. No identity is persisted across backend restarts.
		key := id + ":" + props["object.serial"] + ":" + props["application.process.id"] + ":" + name
		snap.Streams = append(snap.Streams, Stream{ID: id, Key: key, Name: name, Media: props["media.name"], Icon: props["application.icon_name"], OutputID: strconv.FormatUint(uint64(stream.Sink), 10), Volume: stream.Volume.percent(), Muted: stream.Mute, Paused: stream.Corked})
	}
	sort.Slice(snap.Streams, func(i, j int) bool {
		if snap.Streams[i].Name == snap.Streams[j].Name {
			return snap.Streams[i].ID < snap.Streams[j].ID
		}
		return snap.Streams[i].Name < snap.Streams[j].Name
	})
	return snap, nil
}
func (p *Pactl) SetVolume(ctx context.Context, id string, percent int) error {
	_, err := p.command(ctx, "set-sink-input-volume", id, strconv.Itoa(percent)+"%")
	return err
}
func (p *Pactl) SetMute(ctx context.Context, id string, muted bool) error {
	value := "0"
	if muted {
		value = "1"
	}
	_, err := p.command(ctx, "set-sink-input-mute", id, value)
	return err
}
func (p *Pactl) SetDefaultOutput(ctx context.Context, name string) error {
	_, err := p.command(ctx, "set-default-sink", name)
	return err
}
func relevantEvent(line string) bool {
	for _, kind := range []string{"sink-input", "sink", "server", "card"} {
		if strings.Contains(line, " on "+kind+" #") {
			return true
		}
	}
	return false
}
func (p *Pactl) Watch(ctx context.Context, changes chan<- struct{}) error {
	cmd := pactlCommand(ctx, "subscribe")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		if relevantEvent(scanner.Text()) {
			select {
			case changes <- struct{}{}:
			default:
			}
		}
	}
	scanErr := scanner.Err()
	if scanErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if scanErr != nil {
		return scanErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr != nil {
		return fmt.Errorf("audio event connection lost: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return fmt.Errorf("audio event connection closed")
}
