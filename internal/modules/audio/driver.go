// Package audio provides playback-stream controls through PipeWire's
// PulseAudio-compatible service. No raw audio is captured or processed.
package audio

import "context"

type Output struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Volume      int    `json:"volume"`
	Muted       bool   `json:"muted"`
}
type Stream struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Name     string `json:"name"`
	Media    string `json:"media"`
	Icon     string `json:"icon"`
	OutputID string `json:"output_id"`
	Volume   int    `json:"volume"`
	Muted    bool   `json:"muted"`
	Paused   bool   `json:"paused"`
}
type Snapshot struct {
	Revision      uint64   `json:"revision"`
	Available     bool     `json:"available"`
	Error         string   `json:"error,omitempty"`
	Server        string   `json:"server,omitempty"`
	DefaultOutput string   `json:"default_output"`
	Outputs       []Output `json:"outputs"`
	Streams       []Stream `json:"streams"`
}

type Driver interface {
	Read(context.Context) (Snapshot, error)
	SetVolume(context.Context, string, int) error
	SetMute(context.Context, string, bool) error
	SetDefaultOutput(context.Context, string) error
	MoveStream(context.Context, string, string) error
	Watch(context.Context, chan<- struct{}) error
}
