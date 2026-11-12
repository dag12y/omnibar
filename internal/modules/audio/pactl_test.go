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
		case "[--format=json list sources]", "[--format=json list source-outputs]":
			return []byte(`[]`), nil
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
	for _, kind := range []string{"sink-input", "sink", "source", "source-output", "server", "card"} {
		if !relevantEvent("Event 'change' on " + kind + " #12") {
			t.Fatal(kind)
		}
	}
	for _, kind := range []string{"client"} {
		if relevantEvent("Event 'new' on " + kind + " #12") {
			t.Fatal("unrelated event refreshed the mixer: " + kind)
		}
	}
}

func TestPactlParsesMicrophonesMonitorsAndRecordingApps(t *testing.T) {
	p := &Pactl{command: func(_ context.Context, args ...string) ([]byte, error) {
		switch fmt.Sprint(args) {
		case "[--format=json info]":
			return []byte(`{"default_source_name":"mic_a"}`), nil
		case "[--format=json list sinks]", "[--format=json list sink-inputs]":
			return []byte(`[]`), nil
		case "[--format=json list sources]":
			return []byte(`[
   {"index":10,"name":"mic_a","description":"Built-in microphone","mute":true,"volume":{"mono":{"value":16384}},"monitor_of_sink":4294967295},
   {"index":11,"name":"mic_b","volume":{"mono":{"value":65536}},"monitor_of_sink":null},
   {"index":1,"name":"speakers.monitor","properties":{"device.class":"monitor"}},
   {"index":2,"name":"system_audio","monitor_of_sink":0}
  ]`), nil
		case "[--format=json list source-outputs]":
			return []byte(`[
   {"index":30,"source":10,"corked":true,"properties":{"application.name":"Recorder","application.icon_name":"recorder","media.name":"Call"}},
   {"index":31,"source":1,"properties":{}}
  ]`), nil
		}
		return nil, fmt.Errorf("unexpected args %v", args)
	}}
	snap, err := p.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.DefaultInput != "mic_a" || len(snap.Inputs) != 4 || len(snap.Recordings) != 2 {
		t.Fatalf("bad capture metadata: %+v", snap)
	}
	if snap.Inputs[0].Volume != 25 || !snap.Inputs[0].Muted || snap.Inputs[0].Monitor || snap.Inputs[1].Monitor || !snap.Inputs[2].Monitor || !snap.Inputs[3].Monitor {
		t.Fatalf("bad inputs: %+v", snap.Inputs)
	}
	if snap.Recordings[0].Name != "Recorder" || snap.Recordings[0].InputID != "10" || !snap.Recordings[0].Paused || snap.Recordings[1].Name != "Recording app" {
		t.Fatalf("bad recording apps: %+v", snap.Recordings)
	}
}
func TestMicrophoneCommandsAreControlOnly(t *testing.T) {
	var calls [][]string
	p := &Pactl{command: func(_ context.Context, args ...string) ([]byte, error) { calls = append(calls, args); return nil, nil }}
	p.SetInputVolume(context.Background(), "mic name;literal", 30)
	p.SetInputMute(context.Background(), "mic name;literal", true)
	p.SetDefaultInput(context.Background(), "mic name;literal")
	want := [][]string{{"set-source-volume", "mic name;literal", "30%"}, {"set-source-mute", "mic name;literal", "1"}, {"set-default-source", "mic name;literal"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected commands: %v", calls)
	}
}
