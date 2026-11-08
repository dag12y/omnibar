# ADR-0004: Module Execution Model — Hybrid In-Process / Out-of-Process

Status: Accepted

## Context

Omnibar's modules range from read-only system monitors to screen capture and clipboard managers. The original architecture left open whether modules run in-process or as separate services. Running everything in-process risks a single module crash taking down the entire application. Running everything out-of-process adds unnecessary complexity and overhead for simple modules.

## Decision

Use a **hybrid module execution model**:

| Category | Definition | Execution | Examples |
|----------|-----------|-----------|----------|
| Core modules | Part of the core binary; always loaded | In-process | Module registry, settings, logging, IPC server |
| In-process components | Go packages compiled into the core | In-process | Read-only system monitors (/proc, sysfs) |
| Out-of-process services | Separate processes supervised by the core | Separate process | Clipboard manager, audio control, screen capture |

Modules are **not** automatically separate processes. Use process isolation when it provides meaningful reliability or security benefits.

## Alternatives considered

### All in-process

All modules compiled into the core binary, communicating via function calls.

**Advantages:** Simplest to build; no IPC overhead between modules.

**Disadvantages:** A module panic or segfault crashes the entire application; a memory leak in one module affects all others; no security boundary between modules.

**Decision:** Rejected for modules that touch screen, clipboard, audio, or processes. Acceptable for read-only monitors.

### All out-of-process

Every module is a separate process.

**Advantages:** Maximum fault isolation; each module can be restarted independently.

**Disadvantages:** High complexity and overhead; every module needs its own IPC connection, lifecycle management, and error handling; overkill for simple read-only monitors.

**Decision:** Rejected as the default. Out-of-process is opt-in based on need.

### Hybrid (chosen)

Start in-process; move to out-of-process when reliability or security demands it.

**Advantages:** Simple for common cases; fault isolation where it matters; can evolve as needs change.

**Disadvantages:** Two code paths to maintain (in-process and out-of-process); module boundary must be well-defined.

**Decision:** Accepted.

## Module registration

Built-in Go modules use **static registration**. Each module provides a `ModuleInfo` struct (ID, name, version, category, permissions, dependencies, factory function) and registers with the `ModuleRegistry` at compile time.

There is **no dynamic loading of arbitrary Go packages**. Go does not support safe, stable runtime package loading, and dynamic plugins would introduce significant complexity and security risk.

Future external modules, if ever supported, should use **separate executables and manifests**, not Go plugins.

## Module lifecycle

```
Registered → Initialized → Started → Running → Stopped → Disposed
```

- Initialize: read config, allocate resources
- Start: open connections, subscribe to events
- Stop: close connections, release resources
- Dispose: final cleanup

## Crash recovery

**In-process module panic:**

- Go's `recover()` only catches panics in the same goroutine where the deferred recovery function runs. It cannot recover from segmentation faults, bus errors, or arbitrary process crashes.
- Phase 1 recovers panics at controlled module execution boundaries.
- When a panic is recovered: the affected module is marked as failed, the error and stack trace are logged, and dependent functionality is stopped safely.
- Only modules that explicitly support safe reinitialization are restarted. In-process module recovery does not automatically make restarting a module safe — a module that corrupted its own state before panicking may not be safely restartable.

**Out-of-process service crash:**

- The core detects exit via `waitpid`/exit channel and restarts with exponential backoff.
- A crashing service cannot corrupt core memory.

## Health monitoring

The core tracks the state of each module and out-of-process service. Missing heartbeat from an out-of-process service for more than 30 seconds marks it unhealthy and triggers a restart.

## Inter-module communication

All inter-module communication goes through the core (hub-and-spoke): Module A → Core → Module B. No direct module-to-module references, keeping modules decoupled.

## Fault isolation vs security sandboxing

- **Fault isolation** prevents a module crash from taking down the entire application. Implemented in Phase 1 via out-of-process services and panic recovery at module boundaries.
- **Security sandboxing** (OS-level restrictions) is NOT implemented in Phase 1. All modules run as the user. Fault isolation is not a security boundary.

## Consequences

**Positive:**
- Simple default path for most modules
- Fault isolation for risky modules (clipboard, capture, audio)
- No dynamic Go plugin complexity
- Clear upgrade path: in-process modules can be moved out-of-process later

**Negative:**
- Two execution models to document and test
- Module authors must declare their execution category upfront
- In-process panic recovery has limits (cannot catch segfaults)

## References

- Go `os/exec` and `os.Process` for supervising out-of-process services
- Go `defer`/`recover` for in-process panic recovery (same goroutine only)
