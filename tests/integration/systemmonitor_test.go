package integration

import (
	"encoding/json"
	"testing"
	"time"

	"omnibar/internal/ipc/jsonrpc"
)

func TestSystemGetSnapshotDeniedWithoutPermission(t *testing.T) {
	env := newTestEnv(t)
	env.writeConfig(t, `config_version = 1
[permissions]
`)
	p := startBackend(t, env)
	defer p.stopGracefully(t)

	c := dial(t, env.socketPath)
	handshake(t, c)

	resp, _ := c.request("system.getSnapshot", nil)
	if resp.Error == nil || resp.Error.Code != jsonrpc.CodePermissionDenied {
		t.Fatalf("error = %+v, want code %d", resp.Error, jsonrpc.CodePermissionDenied)
	}
	if resp.Error.Module != "system_monitor" {
		t.Errorf("error module = %q, want system_monitor", resp.Error.Module)
	}
}

func TestSystemGetSnapshotGranted(t *testing.T) {
	env := newTestEnv(t)
	env.writeConfig(t, `config_version = 1
[permissions]
"system.info" = true
`)
	p := startBackend(t, env)
	defer p.stopGracefully(t)

	c := dial(t, env.socketPath)
	handshake(t, c)

	resp, _ := c.request("system.getSnapshot", nil)
	if resp.Error != nil {
		t.Fatalf("system.getSnapshot failed: %+v", resp.Error)
	}

	var snap struct {
		CPU      struct {
			Percent float64   `json:"percent"`
			Cores   []float64 `json:"cores"`
		} `json:"cpu"`
		Memory struct {
			Total     uint64 `json:"total"`
			Available uint64 `json:"available"`
			Used      uint64 `json:"used"`
		} `json:"memory"`
		Swap     struct {
			Total uint64 `json:"total"`
			Used  uint64 `json:"used"`
		} `json:"swap"`
		Disk struct {
			Total         uint64 `json:"total"`
			Available     uint64 `json:"available"`
			Used          uint64 `json:"used"`
			ReadBytesSec  uint64 `json:"read_bytes_sec"`
			WriteBytesSec uint64 `json:"write_bytes_sec"`
		} `json:"disk"`
		Network struct {
			RxBytesSec uint64 `json:"rx_bytes_sec"`
			TxBytesSec uint64 `json:"tx_bytes_sec"`
		} `json:"network"`
		SampledAt string `json:"sampled_at"`
	}
	if err := json.Unmarshal(resp.Result, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.SampledAt == "" {
		t.Error("SampledAt must not be empty")
	}
	if snap.CPU.Percent < 0 || snap.CPU.Percent > 100 {
		t.Errorf("CPU.Percent = %f, want 0-100", snap.CPU.Percent)
	}
	if snap.Memory.Total == 0 {
		t.Error("Memory.Total should be non-zero")
	}
	if snap.Memory.Used > snap.Memory.Total {
		t.Errorf("Memory.Used (%d) > Memory.Total (%d)", snap.Memory.Used, snap.Memory.Total)
	}
	if snap.Disk.Total == 0 {
		t.Error("Disk.Total should be non-zero")
	}
}

func TestSystemSubscribeAndEvents(t *testing.T) {
	env := newTestEnv(t)
	env.writeConfig(t, `config_version = 1
[permissions]
"system.info" = true
`)
	p := startBackend(t, env)
	defer p.stopGracefully(t)

	c := dial(t, env.socketPath)
	handshake(t, c)

	resp, _ := c.request("module.subscribe", map[string]any{
		"module": "system_monitor",
		"events": []string{"cpu", "memory"},
	})
	if resp.Error != nil {
		t.Fatalf("subscribe failed: %+v", resp.Error)
	}

	// The sampler runs at the configured interval (default 2s). Wait up to
	// 10s for the first cpu or memory event.
	c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer c.conn.SetReadDeadline(time.Time{})

	for {
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			t.Fatalf("read event: %v", err)
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		var method string
		_ = json.Unmarshal(raw["method"], &method)
		if method != "module.event" {
			continue
		}
		var params struct {
			Module string          `json:"module"`
			Event  string          `json:"event"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw["params"], &params); err != nil {
			t.Fatalf("unmarshal event params: %v", err)
		}
		if params.Module == "system_monitor" && (params.Event == "cpu" || params.Event == "memory") {
			return
		}
	}
}

func TestSystemSubscribeDeniedWithoutPermission(t *testing.T) {
	env := newTestEnv(t)
	env.writeConfig(t, `config_version = 1
[permissions]
`)
	p := startBackend(t, env)
	defer p.stopGracefully(t)

	c := dial(t, env.socketPath)
	handshake(t, c)

	resp, _ := c.request("module.subscribe", map[string]any{
		"module": "system_monitor",
	})
	if resp.Error == nil || resp.Error.Code != jsonrpc.CodePermissionDenied {
		t.Fatalf("error = %+v, want code %d", resp.Error, jsonrpc.CodePermissionDenied)
	}
	if resp.Error.Module != "system_monitor" {
		t.Errorf("error module = %q, want system_monitor", resp.Error.Module)
	}
}
