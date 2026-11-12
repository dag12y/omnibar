package systemmonitor

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"omnibar/internal/config"
	"omnibar/internal/events"
	"omnibar/internal/modules"
)

const (
	// Name is the module's unique registry name.
	Name = "system_monitor"

	// Version is the module version reported in ModuleInfo.
	Version = "0.1.0"

	// Capability is the permission required to read system info.
	Capability = "system.info"

	// EventCPU is the event name for CPU usage samples.
	EventCPU = "cpu"

	// EventMemory is the event name for memory and swap samples.
	EventMemory = "memory"

	// EventDisk is the event name for disk storage and I/O rate samples.
	EventDisk = "disk"

	// EventNetwork is the event name for network transfer rate samples.
	EventNetwork = "network"

	// EventBattery is the event name for battery state and charge samples.
	EventBattery = "battery"
)

var (
	_ modules.Module   = (*Module)(nil)
	_ modules.Lifecycle = (*Module)(nil)
)

// Snapshot is the result of system.getSnapshot and the basis of published
// events.
type Snapshot struct {
	CPU       CPUSnapshot     `json:"cpu"`
	Memory    MemorySnapshot  `json:"memory"`
	Swap      SwapSnapshot    `json:"swap"`
	Disk      DiskSnapshot    `json:"disk"`
	Network   NetworkSnapshot `json:"network"`
	Battery   BatterySnapshot `json:"battery"`
	SampledAt string          `json:"sampled_at"`
}

// CPUSnapshot holds overall and per-core CPU usage as percentages (0-100).
type CPUSnapshot struct {
	Percent float64   `json:"percent"`
	Cores   []float64 `json:"cores,omitempty"`
}

// MemorySnapshot holds memory sizes in bytes.
type MemorySnapshot struct {
	Total     uint64 `json:"total"`
	Available uint64 `json:"available"`
	Used      uint64 `json:"used"`
}

// SwapSnapshot holds swap sizes in bytes.
type SwapSnapshot struct {
	Total uint64 `json:"total"`
	Used  uint64 `json:"used"`
}

// DiskSnapshot holds storage usage and I/O transfer rates in bytes and bytes/sec.
type DiskSnapshot struct {
	Total         uint64 `json:"total"`
	Available     uint64 `json:"available"`
	Used          uint64 `json:"used"`
	ReadBytesSec  uint64 `json:"read_bytes_sec"`
	WriteBytesSec uint64 `json:"write_bytes_sec"`
}

// NetworkSnapshot holds network transfer rates in bytes/sec.
type NetworkSnapshot struct {
	RxBytesSec uint64 `json:"rx_bytes_sec"`
	TxBytesSec uint64 `json:"tx_bytes_sec"`
}

// BatterySnapshot holds battery presence, charge level, status, and power consumption.
type BatterySnapshot struct {
	Present    bool    `json:"present"`
	Percent    float64 `json:"percent"`
	State      string  `json:"state"`
	PowerWatts float64 `json:"power_watts"`
}

// computeSnapshot builds a Snapshot from consecutive readings. When
// prev is nil (first reading), no CPU or disk delta can be computed.
func computeSnapshot(
	stat *StatSample,
	mem *MemInfo,
	prev *StatSample,
	disk *DiskSample,
	prevDisk *DiskSample,
	fsStat FsStatFunc,
	fsPath string,
	net *NetSample,
	prevNet *NetSample,
	battery *BatterySample,
) *Snapshot {
	snap := &Snapshot{
		SampledAt: time.Now().UTC().Format(time.RFC3339),
	}

	// CPU usage from delta between consecutive readings. Guard against
	// counter wrap/reset (uint64 underflow): if the new total is below the
	// previous total, the delta is invalid and percent stays at zero.
	if stat != nil && prev != nil && stat.CPUTimes.Total() >= prev.CPUTimes.Total() {
		totalDelta := stat.CPUTimes.Total() - prev.CPUTimes.Total()
		idleDelta := stat.CPUTimes.IdleTotal() - prev.CPUTimes.IdleTotal()
		if idleDelta > totalDelta {
			idleDelta = totalDelta // clamp idle to total
		}
		if totalDelta > 0 {
			p := float64(totalDelta-idleDelta) / float64(totalDelta) * 100
			snap.CPU.Percent = clampPercent(p)
		}

		// Per-core percentages.
		for i := 0; i < len(stat.Cores) && i < len(prev.Cores); i++ {
			c := stat.Cores[i]
			pc := prev.Cores[i]
			if c.Total() < pc.Total() {
				// Core counter wrapped: report 0 for this core.
				snap.CPU.Cores = append(snap.CPU.Cores, 0)
				continue
			}
			cTotalDelta := c.Total() - pc.Total()
			cIdleDelta := c.IdleTotal() - pc.IdleTotal()
			if cIdleDelta > cTotalDelta {
				cIdleDelta = cTotalDelta
			}
			var cPercent float64
			if cTotalDelta > 0 {
				cPercent = float64(cTotalDelta-cIdleDelta) / float64(cTotalDelta) * 100
			}
			snap.CPU.Cores = append(snap.CPU.Cores, clampPercent(cPercent))
		}
	}

	// Memory: used = total - available (kernel MemAvailable).
	if mem != nil {
		snap.Memory.Total = mem.MemTotal * 1024
		snap.Memory.Available = mem.MemAvailable * 1024
		snap.Memory.Used = (mem.MemTotal - mem.MemAvailable) * 1024

		// Swap: used = total - free; zero totals are valid (no swap).
		snap.Swap.Total = mem.SwapTotal * 1024
		snap.Swap.Used = (mem.SwapTotal - mem.SwapFree) * 1024
	}

	// Disk storage: query filesystem if fsStat is provided.
	if fsStat != nil && fsPath != "" {
		if total, avail, used, err := fsStat(fsPath); err == nil {
			snap.Disk.Total = total
			snap.Disk.Available = avail
			snap.Disk.Used = used
		}
	}

	// Disk I/O throughput delta.
	if disk != nil && prevDisk != nil {
		rRate, wRate := computeDiskDelta(disk, prevDisk)
		snap.Disk.ReadBytesSec = rRate
		snap.Disk.WriteBytesSec = wRate
	}

	// Network throughput delta.
	if net != nil && prevNet != nil {
		rxRate, txRate := computeNetDelta(net, prevNet)
		snap.Network.RxBytesSec = rxRate
		snap.Network.TxBytesSec = txRate
	}

	// Battery status and power metrics.
	if battery != nil && battery.Present {
		snap.Battery.Present = true
		snap.Battery.Percent = battery.Percent
		snap.Battery.State = battery.State
		snap.Battery.PowerWatts = battery.PowerWatts
	}

	return snap
}

func clampPercent(p float64) float64 {
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return math.Round(p*10) / 10
}

// Module implements modules.Module and modules.Lifecycle for system
// monitoring. It owns a sampler goroutine that publishes cpu and memory
// events while the module is running and at least one client is subscribed.
type Module struct {
	config   config.ModuleConfig
	logger   *slog.Logger
	hub      *events.Hub
	publisher *events.Publisher
	sampler  *sampler

	running atomic.Bool
}

// New creates the system_monitor module. interval_ms of 0 means the
// default (config.DefaultIntervalMs).
func New(cfg config.ModuleConfig, logger *slog.Logger, hub *events.Hub) *Module {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	intervalMs := cfg.IntervalMs
	if intervalMs == 0 {
		intervalMs = config.DefaultIntervalMs
	}
	m := &Module{
		config: cfg,
		logger: logger,
		hub:    hub,
	}
	m.publisher = hub.Publisher(Name, m.isRunning)
	m.sampler = newSampler(Name, time.Duration(intervalMs)*time.Millisecond, hub, logger)
	return m
}

// Info returns the module metadata. It declares the system.info
// capability and is enabled by default.
func (m *Module) Info() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:        Name,
		Description: "System monitoring: CPU, memory, and swap.",
		Version:     Version,
		Capabilities: []string{Capability},
		Enabled:     true,
	}
}

// Initialize is a no-op: resources are acquired in Start.
func (m *Module) Initialize(context.Context) error {
	return nil
}

// Start begins sampling. Sampling only produces events while clients are
// subscribed (the sampler checks the hub), so there is no idle cost when
// nobody is listening.
func (m *Module) Start(context.Context) error {
	m.running.Store(true)
	m.sampler.start()
	return nil
}

// Stop halts sampling and releases resources.
func (m *Module) Stop(context.Context) error {
	m.running.Store(false)
	m.sampler.stop()
	return nil
}

// Dispose is a no-op: Stop already released all resources.
func (m *Module) Dispose(context.Context) error {
	return nil
}

// isRunning reports whether the module is in its running state. It is used
// by the events Publisher as a second gate before publishing.
func (m *Module) isRunning() bool {
	return m.running.Load()
}

// Snapshot returns the latest sample. If the sampler has never produced a
// sample (no subscribers yet), it takes one sample on demand using the
// delta since the baseline reading taken at Start.
func (m *Module) Snapshot() *Snapshot {
	if snap := m.sampler.currentSnapshot(); snap != nil {
		return snap
	}
	snap, err := m.sampler.sampleOnce()
	if err != nil {
		m.logger.Error("on-demand sample failed", "module", Name, "error", err)
		return nil
	}
	return snap
}

// marshalCPUEvent returns the JSON payload for a cpu event.
func marshalCPUEvent(snap *Snapshot) json.RawMessage {
	out, _ := json.Marshal(map[string]any{
		"percent": snap.CPU.Percent,
		"cores":   snap.CPU.Cores,
	})
	return out
}

// marshalMemoryEvent returns the JSON payload for a memory event.
func marshalMemoryEvent(snap *Snapshot) json.RawMessage {
	out, _ := json.Marshal(map[string]any{
		"total":      snap.Memory.Total,
		"available":  snap.Memory.Available,
		"used":       snap.Memory.Used,
		"swap_total": snap.Swap.Total,
		"swap_used":  snap.Swap.Used,
	})
	return out
}

// marshalDiskEvent returns the JSON payload for a disk event.
func marshalDiskEvent(snap *Snapshot) json.RawMessage {
	out, _ := json.Marshal(map[string]any{
		"total":           snap.Disk.Total,
		"available":       snap.Disk.Available,
		"used":            snap.Disk.Used,
		"read_bytes_sec":  snap.Disk.ReadBytesSec,
		"write_bytes_sec": snap.Disk.WriteBytesSec,
	})
	return out
}

// marshalNetworkEvent returns the JSON payload for a network event.
func marshalNetworkEvent(snap *Snapshot) json.RawMessage {
	out, _ := json.Marshal(map[string]any{
		"rx_bytes_sec": snap.Network.RxBytesSec,
		"tx_bytes_sec": snap.Network.TxBytesSec,
	})
	return out
}

// marshalBatteryEvent returns the JSON payload for a battery event.
func marshalBatteryEvent(snap *Snapshot) json.RawMessage {
	out, _ := json.Marshal(map[string]any{
		"present":     snap.Battery.Present,
		"percent":     snap.Battery.Percent,
		"state":       snap.Battery.State,
		"power_watts": snap.Battery.PowerWatts,
	})
	return out
}
