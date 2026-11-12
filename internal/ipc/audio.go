package ipc

import (
	"context"
	"encoding/json"
	"time"

	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/audio"
)

func RegisterAudioMethods(d *jsonrpc.Dispatcher, registry *modules.Registry, mixer *audio.Module) {
	for _, method := range []string{"audio.getSnapshot", "audio.setVolume", "audio.setMute", "audio.setDefaultOutput"} {
		d.RegisterContext(method, func(ctx context.Context, params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
			if !registry.IsEnabled(audio.Name) {
				return nil, jsonrpc.NewError(jsonrpc.CodeModuleUnavailable, "Audio is disabled", audio.Name)
			}
			if !registry.HasCapability(audio.Name, audio.Capability) {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Permission denied: audio.control", audio.Name)
			}
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			var snap audio.Snapshot
			if method == "audio.getSnapshot" {
				snap = mixer.Snapshot(ctx)
			} else {
				var p struct {
					Key    string `json:"key"`
					Volume *int   `json:"volume"`
					Muted  *bool  `json:"muted"`
					Output string `json:"output"`
				}
				invalid := func() (json.RawMessage, *jsonrpc.Error) {
					return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid audio parameters", audio.Name)
				}
				if err := json.Unmarshal(params, &p); err != nil {
					return invalid()
				}
				action, key, volume, muted := "", p.Key, 0, false
				switch method {
				case "audio.setVolume":
					if key == "" || p.Volume == nil || *p.Volume < 0 || *p.Volume > 100 {
						return invalid()
					}
					action, volume = "volume", *p.Volume
				case "audio.setMute":
					if key == "" || p.Muted == nil {
						return invalid()
					}
					action, muted = "mute", *p.Muted
				case "audio.setDefaultOutput":
					if p.Output == "" {
						return invalid()
					}
					action, key = "output", p.Output
				}
				var err error
				snap, err = mixer.Change(ctx, action, key, volume, muted)
				if err != nil {
					return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, err.Error(), audio.Name)
				}
			}
			out, _ := json.Marshal(snap)
			return out, nil
		})
	}
}
