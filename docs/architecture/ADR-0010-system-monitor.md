# ADR-0010: System Monitoring Module (CPU, Memory, Swap)

Status: Accepted

## Context

Phase 2 adds system monitoring. ARCHITECTURE.md classifies read-only monitors that read `/proc` and sysfs as in-process components of the core binary, gated by the `system.info` capability (ADR-0007). This ADR covers the first monitor, CPU plus memory and swap, and sets the pattern later monitors (disk, network, battery, GPU) will follow.

## Decision

### Module

- Name: `system_monitor`. One built-in, statically registered module (ADR-0004); later monitors extend it rather than adding a module each.
- Declares one capability: `system.info`. Methods and subscriptions are denied unless the user grants it. With no grant the module registers but is unusable (fail-closed).
- Enabled by default, like all built-in modules; it can be turned off with `[modules.system_monitor] enabled = false`.

### Data sources

| Metric | Source | Computation |
|--------|--------|-------------|
| CPU usage | `/proc/stat` | Delta of idle and total jiffies between two samples, as a percentage; also per core |
| Memory | `/proc/meminfo` | `total`, `available`, `used = total - available` (kernel `MemAvailable`) |
| Swap | `/proc/meminfo` | `total`, `free`, `used = total - free`; zero totals are valid (no swap) |

Reads use plain files, no external commands and no new dependencies. Parsers take an `io.Reader` or a root directory so tests use fixture files and never depend on the real `/proc`.

### RPC surface

- `system.getSnapshot` (request, no params). Returns the latest values:

  ```json
  {"cpu":{"percent":4.2,"cores":[3.1,5.0]},"memory":{"total":16000000000,"available":9000000000,"used":7000000000},"swap":{"total":0,"used":0},"sampled_at":"2026-01-01T12:00:00Z"}
  ```

  Sizes are bytes. Percentages are 0-100 with one decimal. Requires the `system.info` grant, otherwise error -32001.

- Events (ADR-0009), published only while subscribed:
  - `cpu`: `{"percent":4.2,"cores":[...]}`
  - `memory`: `{"total":...,"available":...,"used":...,"swap_total":...,"swap_used":...}`

### Sampling

- A sampler goroutine runs only while the module is running and at least one client is subscribed to one of its events (ADR-0009 on-demand production). With no subscribers it does nothing, so idle cost is zero.
- A baseline `/proc/stat` read is taken when the sampler starts so the first published value is a real delta, not an average since boot.
- `system.getSnapshot` returns the latest sample. If the sampler is idle, it takes one sample on demand, using the delta since the previous read.
- Interval: `interval_ms`, default 2000, valid range 500-60000, validated at configuration load. An invalid value fails validation like any other bad configuration (fail-closed, startup refuses it).
- The sampler stops cleanly on `Stop` and does not leak goroutines.

### Configuration

```toml
[modules.system_monitor]
enabled = true
interval_ms = 2000

[permissions]
"system.info" = true
```

`config.toml.example` already grants `system.info`. Module-specific settings are added to the module's configuration table and validated with the rest of the configuration. The exact Go representation is decided in M10; it must keep `modules.<name>.enabled` working unchanged.

### Scope boundaries

- No history, graphs over time, or alerts. Those need storage and are decided with SQLite (ADR-0006).
- No per-process data (needs `process.manage`, a higher-risk capability).
- No GPU, disk, network, or battery in this ADR. Each is added later behind the same module and capability, with its own parser and tests.

## Alternatives considered

### Shell out to `top`, `free`, `vmstat`

Rejected: process spawning cost, output parsing fragility, and an external dependency for data the kernel exposes directly.

### Third-party library (e.g. gopsutil)

Rejected for now: pulls a large dependency for two files. Revisit when a metric is awkward to read directly (GPU, for example).

### Sample continuously and cache

Rejected: wastes CPU when nobody is looking, and the architecture sets an idle CPU target below 1%.

### One module per metric

Rejected: many modules to register and configure for one subject. One module with several event types matches how users think about "system info".

## Consequences

**Positive:**

- First real consumer of capabilities and events, and a pattern for later monitors.
- Near-zero idle cost.
- Easy to test: pure parsers over fixtures.

**Negative:**

- `/proc` formats are Linux-specific (acceptable: the target is Omarchy).
- CPU percentage depends on the sampling interval; very short intervals are noisy, hence the 500 ms floor.
- No history until storage exists.

## References

- ADR-0004: Module execution model
- ADR-0006: Storage
- ADR-0007: Permissions and security
- ADR-0009: Event subscription and delivery
