package audio

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestPactlParsesPlaybackAndBluetoothOutputs(t *testing.T) {
	p := &Pactl{command: func(_ context.Context, args ...string) ([]byte, error) {
		switch fmt.Sprint(args) {
		case "[--format=json info]":
			return []byte(`{"server_name":"PulseAudio (on PipeWire)","default_sink_name":"headphones"}`), nil
		case "[--format=json list sinks]":
			return []byte(`[{"index":2,"name":"headphones","description":"Bluetooth headphones","mute":false,"volume":{"left":{"value":32768},"right":{"value":32768}}}]`), nil
		case "[--format=json list sink-inputs]":
			return []byte(`[{"index":10,"sink":2,"mute":true,"corked":true,"volume":{"left":{"value":65536},"right":{"value":32768}},"properties":{"application.name":"Music","application.process.id":"100","application.icon_name":"music","media.name":"Song","object.serial":"88"}},{"index":11,"sink":2,"volume":{},"properties":{}}]`), nil
		}
		return nil, fmt.Errorf("unexpected args: %v", args)
	}}
	snap, err := p.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Available || snap.DefaultOutput != "headphones" || snap.Outputs[0].Volume != 50 {
		t.Fatalf("bad outputs: %+v", snap)
	}
	if len(snap.Streams) != 2 || snap.Streams[1].Name != "Music" || snap.Streams[1].Volume != 75 || !snap.Streams[1].Muted || !snap.Streams[1].Paused || snap.Streams[1].Key != "10:88:100:Music" {
		t.Fatalf("bad streams: %+v", snap.Streams)
	}
	if snap.Streams[0].Name != "Audio stream" {
		t.Fatal("missing fallback name")
	}
}
func TestPactlRejectsMalformedState(t *testing.T) {
	p := &Pactl{command: func(context.Context, ...string) ([]byte, error) { return []byte(`not json`), nil }}
	if _, err := p.Read(context.Background()); err == nil {
		t.Fatal("accepted malformed server JSON")
	}
}
func TestPactlMutationsUseArgumentVectors(t *testing.T) {
	var calls [][]string
	p := &Pactl{command: func(_ context.Context, args ...string) ([]byte, error) { calls = append(calls, args); return nil, nil }}
	p.SetVolume(context.Background(), "10", 42)
	p.SetMute(context.Background(), "10", true)
	p.SetDefaultOutput(context.Background(), "output with spaces;literal")
	p.MoveStream(context.Background(), "10", "headphones;literal")
	want := [][]string{{"set-sink-input-volume", "10", "42%"}, {"set-sink-input-mute", "10", "1"}, {"set-default-sink", "output with spaces;literal"}, {"move-sink-input", "10", "headphones;literal"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v", calls)
	}
}
func TestRelevantEventsExcludeOwnClientConnections(t *testing.T) {
	for _, kind := range []string{"sink-input", "sink", "server", "card"} {
		if !relevantEvent("Event 'change' on " + kind + " #12") {
			t.Fatal(kind)
		}
	}
	for _, kind := range []string{"client", "source", "source-output"} {
		if relevantEvent("Event 'new' on " + kind + " #12") {
			t.Fatal("unrelated event refreshed the mixer: " + kind)
		}
	}
}
