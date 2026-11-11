package systemmonitor

import (
	"log/slog"
	"sync"
	"time"

	"omnibar/internal/events"
)

// sampler drives on-demand sampling of /proc/stat and /proc/meminfo.
//
// The run loop ticks at the configured interval and only samples (and
// publishes) when the hub has at least one subscriber for this module.
// With no subscribers it does nothing, keeping idle cost near zero.
// Baseline readings are taken when the sampler starts so the first
// published value is a real delta, not an average since boot.
type sampler struct {
	module      string
	hub         *events.Hub
	interval    time.Duration
	logger      *slog.Logger
	statPath    string
	meminfoPath string
	diskPath    string
	netdevPath  string
	fsPath      string
	fsStat      FsStatFunc

	mu      sync.Mutex
	stopCh  chan struct{}
	done    chan struct{}
	started bool

	lastStat *StatSample
	lastMem  *MemInfo
	lastDisk *DiskSample
	lastNet  *NetSample
	snapshot *Snapshot
}

func newSampler(module string, interval time.Duration, hub *events.Hub, logger *slog.Logger) *sampler {
	return &sampler{
		module:      module,
		hub:         hub,
		interval:    interval,
		logger:      logger,
		statPath:    "/proc/stat",
		meminfoPath: "/proc/meminfo",
		diskPath:    "/proc/diskstats",
		netdevPath:  "/proc/net/dev",
		fsPath:      "/",
		fsStat:      DefaultFsStat,
	}
}

// start launches the sampling goroutine. Safe to call repeatedly.
func (s *sampler) start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.started = true
	s.stopCh = make(chan struct{})
	s.done = make(chan struct{})
	go s.run()
}

// stop signals the goroutine to exit and waits for it. Safe to call
// repeatedly and safe after start has never been called.
func (s *sampler) stop() {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.started = false
	stopCh := s.stopCh
	done := s.done
	s.mu.Unlock()
	close(stopCh)
	<-done
}

// run is the sampling loop. It takes a baseline reading, then ticks at the
// interval. On each tick it samples and publishes if there are subscribers.
// A nil hub means no event delivery: the loop only waits for stop.
func (s *sampler) run() {
	defer close(s.done)

	if s.hub == nil {
		<-s.stopCh
		return
	}

	s.takeBaseline()

	watcher := s.hub.Watch(s.module)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-watcher:
			// Subscription count changed; re-evaluate below.
		case <-ticker.C:
		}

		if !s.hub.HasSubscribers(s.module) {
			continue
		}

		snap, err := s.sample()
		if err != nil {
			s.logger.Error("sample failed", "module", s.module, "error", err)
			continue
		}
		s.publish(snap)
	}
}

// takeBaseline reads /proc/stat, /proc/meminfo, and /proc/diskstats once to establish the
// delta baseline. Errors are ignored: the first real sample will still
// produce a usable (possibly zero) result.
func (s *sampler) takeBaseline() {
	if stat, err := ParseStatFile(s.statPath); err == nil {
		s.mu.Lock()
		s.lastStat = stat
		s.mu.Unlock()
	}
	if mem, err := ParseMeminfoFile(s.meminfoPath); err == nil {
		s.mu.Lock()
		s.lastMem = mem
		s.mu.Unlock()
	}
	if disk, err := ParseDiskstatsFile(s.diskPath, time.Now()); err == nil {
		s.mu.Lock()
		s.lastDisk = disk
		s.mu.Unlock()
	}
	if net, err := ParseNetdevFile(s.netdevPath, time.Now()); err == nil {
		s.mu.Lock()
		s.lastNet = net
		s.mu.Unlock()
	}
}

// sample reads /proc/stat, /proc/meminfo, /proc/diskstats, and /proc/net/dev, computes
// deltas from previous readings, stores the snapshot, and returns it.
func (s *sampler) sample() (*Snapshot, error) {
	stat, err := ParseStatFile(s.statPath)
	if err != nil {
		return nil, err
	}
	mem, err := ParseMeminfoFile(s.meminfoPath)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	disk, _ := ParseDiskstatsFile(s.diskPath, now)
	net, _ := ParseNetdevFile(s.netdevPath, now)

	s.mu.Lock()
	defer s.mu.Unlock()

	snap := computeSnapshot(stat, mem, s.lastStat, disk, s.lastDisk, s.fsStat, s.fsPath, net, s.lastNet)
	s.lastStat = stat
	s.lastMem = mem
	if disk != nil {
		s.lastDisk = disk
	}
	if net != nil {
		s.lastNet = net
	}
	s.snapshot = snap
	return snap, nil
}

// sampleOnce takes a single sample without publishing. It is used for
// on-demand system.getSnapshot when no sample exists yet. It updates the
// baseline so subsequent delta-based samples remain consistent.
func (s *sampler) sampleOnce() (*Snapshot, error) {
	stat, err := ParseStatFile(s.statPath)
	if err != nil {
		return nil, err
	}
	mem, err := ParseMeminfoFile(s.meminfoPath)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	disk, _ := ParseDiskstatsFile(s.diskPath, now)
	net, _ := ParseNetdevFile(s.netdevPath, now)

	s.mu.Lock()
	defer s.mu.Unlock()

	snap := computeSnapshot(stat, mem, s.lastStat, disk, s.lastDisk, s.fsStat, s.fsPath, net, s.lastNet)
	s.lastStat = stat
	s.lastMem = mem
	if disk != nil {
		s.lastDisk = disk
	}
	if net != nil {
		s.lastNet = net
	}
	s.snapshot = snap
	return snap, nil
}

// currentSnapshot returns the latest snapshot, or nil if none has been taken.
func (s *sampler) currentSnapshot() *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}

// publish sends cpu, memory, disk, and network events for snap to subscribed clients.
func (s *sampler) publish(snap *Snapshot) {
	cpuData := marshalCPUEvent(snap)
	if err := s.hub.Publish(s.module, EventCPU, cpuData); err != nil {
		s.logger.Error("publish cpu event", "module", s.module, "error", err)
	}

	memData := marshalMemoryEvent(snap)
	if err := s.hub.Publish(s.module, EventMemory, memData); err != nil {
		s.logger.Error("publish memory event", "module", s.module, "error", err)
	}

	diskData := marshalDiskEvent(snap)
	if err := s.hub.Publish(s.module, EventDisk, diskData); err != nil {
		s.logger.Error("publish disk event", "module", s.module, "error", err)
	}

	netData := marshalNetworkEvent(snap)
	if err := s.hub.Publish(s.module, EventNetwork, netData); err != nil {
		s.logger.Error("publish network event", "module", s.module, "error", err)
	}
}
