package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"omnibar/internal/config"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/audio"
)

type audioStub struct{ reads, changes int }

func (s *audioStub) Read(context.Context) (audio.Snapshot, error) {
	s.reads++
	return audio.Snapshot{Available: true, Outputs: []audio.Output{{ID: "1", Name: "speaker"}}, Streams: []audio.Stream{{ID: "8", Key: "live", Name: "App"}}, Inputs: []audio.Input{{ID: "11", Name: "mic"}}}, nil
}
func (s *audioStub) SetVolume(context.Context, string, int) error      { s.changes++; return nil }
func (s *audioStub) SetMute(context.Context, string, bool) error       { s.changes++; return nil }
func (s *audioStub) SetDefaultOutput(context.Context, string) error    { s.changes++; return nil }
func (s *audioStub) MoveStream(context.Context, string, string) error  { s.changes++; return nil }
func (s *audioStub) SetInputVolume(context.Context, string, int) error { s.changes++; return nil }
func (s *audioStub) SetInputMute(context.Context, string, bool) error  { s.changes++; return nil }
func (s *audioStub) SetDefaultInput(context.Context, string) error     { s.changes++; return nil }
func (s *audioStub) Watch(ctx context.Context, _ chan<- struct{}) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestAudioRPCCapabilitiesAndValidation(t *testing.T) {
	for _, state := range []string{"denied", "disabled", "granted"} {
		t.Run(state, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Permissions[audio.Capability] = state != "denied"
			if state == "disabled" {
				cfg.Modules[audio.Name] = config.ModuleConfig{Enabled: false}
			}
			driver := &audioStub{}
			m := audio.New(nil, driver)
			registry := modules.NewRegistry(cfg, nil)
			registry.Register(m)
			registry.Initialize(context.Background())
			registry.Start(context.Background())
			defer registry.Stop(context.Background())
			d := jsonrpc.NewDispatcher()
			RegisterAudioMethods(d, registry, m)
			cases := []struct {
				method, params string
				valid          bool
			}{
				{"audio.getSnapshot", `{}`, true},
				{"audio.setVolume", `{"key":"live","volume":45}`, true},
				{"audio.setMute", `{"key":"live","muted":false}`, true},
				{"audio.setDefaultOutput", `{"output":"speaker"}`, true},
				{"audio.moveStream", `{"key":"live","output":"speaker"}`, true},
				{"audio.setInputVolume", `{"input":"mic","volume":40}`, true},
				{"audio.setInputMute", `{"input":"mic","muted":false}`, true},
				{"audio.setDefaultInput", `{"input":"mic"}`, true},
				{"audio.setInputVolume", `{"input":"mic","volume":101}`, false},
				{"audio.setInputVolume", `{"input":"mic","volume":-1}`, false},
				{"audio.setInputVolume", `{"input":"mic","volume":1.5}`, false},
				{"audio.setInputVolume", `{"input":"mic"}`, false},
				{"audio.setInputMute", `{"input":"mic"}`, false},
				{"audio.setDefaultInput", `{}`, false},
				{"audio.setDefaultInput", `{"input":42}`, false},
				{"audio.moveStream", `{"output":"speaker"}`, false},
				{"audio.moveStream", `{"key":"live"}`, false},
				{"audio.moveStream", `{"key":"live","output":null}`, false},
				{"audio.moveStream", `{"key":"live","output":42}`, false},
				{"audio.setVolume", `{"key":"live","volume":101}`, false},
				{"audio.setVolume", `{"key":"live","volume":-1}`, false},
				{"audio.setVolume", `{"key":"live","volume":1.5}`, false},
				{"audio.setVolume", `{"key":"live","volume":"50"}`, false},
				{"audio.setVolume", `{"key":"live"}`, false},
				{"audio.setMute", `{"key":"live"}`, false},
				{"audio.setDefaultOutput", `{}`, false},
			}
			for _, tc := range cases {
				raw, err := d.Handle([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`, tc.method, tc.params)))
				if err != nil {
					t.Fatal(err)
				}
				var response jsonrpc.Response
				if err := json.Unmarshal(raw, &response); err != nil {
					t.Fatal(err)
				}
				code := 0
				switch state {
				case "denied":
					code = jsonrpc.CodePermissionDenied
				case "disabled":
					code = jsonrpc.CodeModuleUnavailable
				case "granted":
					if !tc.valid {
						code = jsonrpc.CodeInvalidParams
					}
				}
				if code == 0 {
					if response.Error != nil {
						t.Fatalf("%s: %s", tc.method, raw)
					}
				} else if response.Error == nil || response.Error.Code != code {
					t.Fatalf("%s %s: %s", tc.method, tc.params, raw)
				}
			}
			if state != "granted" && driver.reads != 0 {
				t.Fatal("denied request contacted audio service")
			}
			if state == "granted" && driver.changes != 7 {
				t.Fatalf("mutations = %d", driver.changes)
			}
		})
	}
}
