package integration

import (
	"testing"

	"omnibar/internal/ipc/jsonrpc"
)

func TestAudioFailClosedOverRealSocket(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		name := "missing grant"
		if disabled {
			name = "disabled"
		}
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t)
			if disabled {
				env.writeConfig(t, "[modules.audio]\nenabled = false\n[permissions]\n\"audio.control\" = true\n")
			}
			startBackend(t, env)
			c := dial(t, env.socketPath)
			for _, method := range []string{"audio.getSnapshot", "audio.setVolume", "audio.setMute", "audio.setDefaultOutput", "audio.moveStream"} {
				resp, _ := c.request(method, map[string]any{"key": "anything", "volume": 50, "muted": true, "output": "anything"})
				want := jsonrpc.CodePermissionDenied
				if disabled {
					want = jsonrpc.CodeModuleUnavailable
				}
				if resp.Error == nil || resp.Error.Code != want {
					t.Fatalf("%s: %+v", method, resp.Error)
				}
			}
		})
	}
}
