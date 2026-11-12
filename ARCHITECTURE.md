# Omnibar Architecture

Omnibar is a modular desktop utility suite for Linux, initially targeting Omarchy and Hyprland. This document describes the finalized architecture. Detailed decisions are recorded in Architecture Decision Records (ADRs) under `docs/architecture/`.

## Status

Finalized. See `docs/architecture/ADR-0002` through `ADR-0010` for individual decisions.

## Overview

Omnibar is split into three layers:

1. **Desktop interface** — Quickshell + QML. Responsible for the main control center, floating panels, widgets, notifications, and user interaction.
2. **Core services** — Go. Responsible for module lifecycle, settings, inter-module communication, logging, updates, and permissions.
3. **Linux integration** — Native Linux interfaces (Hyprland IPC, PipeWire, D-Bus, systemd, /proc, sysfs).

The UI and core services are separate processes. They communicate over a Unix domain socket using newline-delimited JSON-RPC 2.0. Go owns the socket server; the QML UI connects as a client using `Quickshell.Io.Socket`.

## Core technology decisions

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| UI | Quickshell + QML | Installed on Omarchy, dependency of the `omarchy` package, purpose-built for desktop shells on Wayland |
| Core services | Go 1.27 | Static binary, low overhead, strong concurrency, `log/slog` in stdlib |
| UI↔Backend IPC | JSON-RPC 2.0 over Unix domain socket (newline-delimited) | Quickshell `Quickshell.Io.Socket` QML type available; clean process boundary; failure isolation |
| Configuration | TOML | Human-editable, supports comments, standard in Linux desktop configs |
| Persistent storage | Bounded JSON for explicit clipboard favorites; SQLite (WAL) deferred for richer data | Private atomic file now; database only when needed |
| Database driver | `modernc.org/sqlite` (pure Go) — evaluated, not introduced in Phase 1 | No CGO, simpler cross-compilation and builds |
| Desktop integration | Native Hyprland IPC | Zero extra dependencies, canonical Hyprland control interface |
| Audio | PipeWire | Standard on Omarchy, `pw-cli`/`pw-dump` available |
| Process management | systemd user services | `systemctl --user`, standard on Arch |
| Logging | Go `log/slog` | Stdlib, structured, no dependency |
| Packaging | Arch PKGBUILD + Omarchy Package Repository | Matches `hype`/`monologue`/`omacut` pattern |

These decisions are locked. Changes require strong technical evidence and an updated ADR.

## Process and UI lifecycle

### Backend startup

The Go backend is launched first. It:

1. Reads configuration from `$XDG_CONFIG_HOME/omnibar/config.toml`.
2. Creates the runtime socket directory `$XDG_RUNTIME_DIR/omnibar/` with mode `0700`.
3. Binds the Unix domain socket `$XDG_RUNTIME_DIR/omnibar/omnibar.sock` with mode `0600`.
4. Initializes the module registry and starts enabled modules.
5. Listens for incoming socket connections from the UI.

### UI launch

The Quickshell UI is launched via the `quickshell` binary, which is installed on Omarchy. Based on the `quickshell --help` output:

```
quickshell [OPTIONS] [SUBCOMMAND]

OPTIONS:
  -c, --config TEXT    Name of a quickshell configuration to run.
  -p, --path TEXT      Path to a QML file or config folder.
  -n, --no-duplicate   Exit immediately if another instance of the given config is running.
  -d, --daemonize      Detach from the controlling terminal.
  -v, --verbose        Increases log verbosity.
```

Quickshell detects configurations as named directories under each XDG config directory as `<xdg dir>/quickshell/<config name>/shell.qml`. If `~/.config/quickshell/shell.qml` exists, it is registered as the 'default' configuration.

The Omnibar UI can be launched with `quickshell -c omnibar` (if installed as a Quickshell config) or `quickshell -p <path-to-omnibar-shell.qml>`. The exact launch command is determined at packaging time and is not part of this architecture document.

### Backend lifetime vs UI lifetime

- The Go backend is a long-running daemon. It stays alive when the UI closes.
- If the UI closes, the backend continues running and retains its socket connection state.
- If the backend exits, all connected UIs lose their socket connection. Each UI enters offline mode and attempts reconnection with exponential backoff.
- The backend does not restart automatically when the UI closes; it is managed independently (e.g., via a systemd user service).

### Runtime socket directory

- Directory: `$XDG_RUNTIME_DIR/omnibar/`, mode `0700`, owned by the running user.
- Created by the Go backend before binding the socket.
- If `$XDG_RUNTIME_DIR` is unset or unavailable, the backend logs a fatal error and exits. It does not fall back to an insecure location (e.g., `/tmp`).

### Stale socket handling

- Before binding, the backend checks whether a socket file already exists at the socket path.
- If a socket file exists, the backend attempts to connect to it. If the connection succeeds, a live backend is already listening; the new instance logs an error and exits to avoid conflicts.
- If the connection fails (e.g., `ECONNREFUSED`), the file is a stale socket left by a crashed process. The backend removes only that specific socket file path after confirming it is a socket (not an arbitrary filesystem entry) and then binds.
- The backend never blindly deletes an arbitrary filesystem entry at the socket path.

### Graceful shutdown

- On `SIGTERM` or `SIGINT`, the backend stops accepting new connections, closes existing client connections, stops all modules, removes the socket file, and exits with code 0.
- On `SIGKILL`, the socket file may be left behind; the stale-socket handling above manages this on the next startup.

### systemd user service

A normal user service unit `omnibar.service` (not a templated `@.service`) is appropriate for the initial architecture because there is one backend instance per user. The unit:

- Runs the Go backend as the user.
- Is enabled and started with `systemctl --user enable --now omnibar`.
- Restarts the backend on failure with exponential backoff.
- Does not manage the UI lifecycle; the UI is launched separately by the user or session.

## Module architecture (hybrid)

Omnibar uses a **hybrid module architecture**. Modules are not automatically separate processes.

| Category | Definition | Execution | Examples |
|----------|-----------|-----------|----------|
| Core modules | Part of the core binary; always loaded | In-process | Module registry, settings, logging, IPC server |
| In-process components | Go packages compiled into the core | In-process | Read-only system monitors (/proc, sysfs) |
| Out-of-process services | Separate processes supervised by the core | Separate process | Clipboard manager, audio control, screen capture |

### Module registration

Built-in Go modules use **static registration**. Each module provides a `ModuleInfo` struct (ID, name, version, category, permissions, dependencies, factory) and registers with the `ModuleRegistry` at compile time. There is no dynamic loading of arbitrary Go packages.

Future external modules, if ever supported, should use separate executables and manifests — not Go plugins.

### Module lifecycle

```
Registered → Initialized → Started → Running → Stopped → Disposed
```

- Initialize: read config, allocate resources
- Start: open connections, subscribe to events
- Stop: close connections, release resources
- Dispose: final cleanup

### Crash recovery

**In-process module panic:**

- `recover()` only catches panics in the same goroutine where the deferred recovery function runs. It cannot recover from segmentation faults or arbitrary process crashes.
- Phase 1 recovers panics at controlled module execution boundaries.
- When a panic is recovered: the affected module is marked as failed, the error and stack trace are logged, and dependent functionality is stopped safely.
- Only modules that explicitly support safe reinitialization are restarted. In-process module recovery does not automatically make restarting a module safe.

**Out-of-process service crash:**

- The core detects exit via `waitpid`/exit channel and restarts with exponential backoff.
- A crashing service cannot corrupt core memory.

### Current shipped utility modules

`system_monitor`, `keep_awake`, `audio`, and `clipboard` are static in-process Go modules.
System sampling and audio watching follow client demand; Keep Awake owns an
explicit timed lease independently of visible pages. Audio uses short `pactl`
commands and a cancellable `pactl subscribe` child process against PipeWire's
PulseAudio compatibility service. Its snapshots, actions, permissions, retries,
and UI behavior are specified in [ADR-0012](docs/architecture/ADR-0012-audio-mixer.md).
Per-stream output routing extends it in
[ADR-0013](docs/architecture/ADR-0013-stream-routing.md), using the same capability
and revisioned snapshots. Microphone gain/mute/default-input and recording-app
metadata extend the same module in [ADR-0014](docs/architecture/ADR-0014-microphone-controls.md).
Microphone samples are not captured; input gain is metadata, and recording apps
are the audio server's source-output list. This is the implemented audio path; a separate supervised audio service remains
a possible future design. Keep Awake is described in [ADR-0011](docs/architecture/ADR-0011-keep-awake.md).

Clipboard uses an explicitly started `wl-paste --watch` child and a bounded
internal capture helper, with in-memory history and separate read/write grants.
The UI polls only while visible; collection follows the explicit session toggle,
independently of panel visibility. This supersedes the planned persistent or
separate-service clipboard path for this increment. See
[ADR-0015](docs/architecture/ADR-0015-clipboard-history.md). Explicitly saved
favorites extend it in [ADR-0016](docs/architecture/ADR-0016-clipboard-favorites.md),
using bounded private JSON, atomic replacement, and no ordinary-history persistence.

### Inter-module communication

All inter-module communication goes through the core (hub-and-spoke). Module A → Core → Module B. No direct module-to-module references, keeping modules decoupled.

## IPC protocol

Go owns the Unix domain socket server. The QML UI connects as a client using `Quickshell.Io.Socket` from `Quickshell.Io`. `SocketServer` is not used for this direction of communication.

### Socket security

- **Socket directory:** `$XDG_RUNTIME_DIR/omnibar/`, created with mode `0700`, owned by the running user.
- **Socket file:** `$XDG_RUNTIME_DIR/omnibar/omnibar.sock`, created with mode `0600`, owned by the running user.
- No separate authentication: single-user desktop, socket in the user's runtime directory. The user running the UI is the same user running the backend.
- If `$XDG_RUNTIME_DIR` is unset or unavailable, the backend logs a fatal error and exits.

### Message framing

- Each message is exactly one JSON object, terminated by a single `\n` (newline-delimited).
- The receiver reads bytes until `\n`, then parses the JSON object.
- Maximum message size is bounded (default 1 MiB). Messages exceeding the limit are rejected and the connection is closed.

This framing prevents unbounded memory growth from a malformed or malicious sender and makes stream parsing deterministic.

### JSON-RPC 2.0 and handshake

**Every request has an ID.** Its response uses the same ID. A JSON-RPC response must **not** contain a `method` field. If the backend initiates a request toward the UI, it must use its own request ID.

**Handshake request (UI → backend):**
```json
{"jsonrpc":"2.0","id":1,"method":"client.handshake","params":{"protocol":"omnibar.ipc/v1","client_id":"omnibar-ui-1"}}
```

**Handshake response (backend → UI):**
```json
{"jsonrpc":"2.0","id":1,"result":{"protocol":"omnibar.ipc/v1","version":"0.1.0","modules":["system_monitor"]}}
```

**Handshake error — protocol version incompatibility (backend → UI):**
```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"Protocol version mismatch: expected omnibar.ipc/v1, got omnobar.ipc/v2"}}
```

**Request/response example:**
```json
// Request (UI → backend)
{"jsonrpc":"2.0","id":42,"method":"module.getStatus","params":{"module":"system_monitor"}}

// Successful response (backend → UI, same id, no method field)
{"jsonrpc":"2.0","id":42,"result":{"running":true,"cpu":4.2}}

// Error response (backend → UI, same id, no method field)
{"jsonrpc":"2.0","id":42,"error":{"code":-32600,"message":"Module not found","module":"system_monitor"}}
```

**Notification (backend → UI, no `id`):**
```json
{"jsonrpc":"2.0","method":"module.event","params":{"module":"audio","event":"snapshot","data":{"revision":1,"available":true,"default_output":"","outputs":[],"streams":[]}}}
```

### Connection lifecycle

1. UI starts and attempts to connect to `$XDG_RUNTIME_DIR/omnibar/omnibar.sock`.
2. If the backend is not running, the UI shows a "Connecting…" state and retries with exponential backoff (1s, 2s, 4s, 8s, 16s, then capped at 30s).
3. On connect, the UI sends `client.handshake` with client ID and protocol version (`omnibar.ipc/v1`).
4. The backend replies with `server.handshake` including protocol version and available modules.
5. The UI subscribes to events of interest via `module.subscribe`.
6. The backend pushes notifications for subscribed events.

Subscription methods, error codes, delivery guarantees, and the slow-client policy are specified in ADR-0009. Delivery is at most once with no replay, so after a reconnect the UI re-subscribes and then fetches current state with a request.

### Reconnection behavior

- Socket disconnection → UI enters offline mode: cached data displayed, backend-requiring actions disabled with a "Backend unavailable" message.
- Automatic reconnect attempts with exponential backoff.
- On reconnect, UI re-sends handshake, re-subscribes to events, then fetches current state (ADR-0009).
- Pending requests that timed out are reported as errors to the UI.

### Error handling

- **Standard JSON-RPC error codes** (-32700 through -32603) are reserved for standard protocol errors: parse error, invalid request, method not found, invalid params, internal error.
- **Server-defined custom errors** use the range -32000 through -32999.
- Backend errors include a `module` field for routing when applicable.
- UI shows non-blocking error toasts for transient errors; critical errors are shown inline.

### Protocol versioning

- Handshake exchanges protocol version `omnibar.ipc/v1`.
- Mismatched major versions are rejected with a standard JSON-RPC error (code -32600).
- Minor version differences are tolerated with deprecation warnings.

### Graceful degradation

The UI must continue functioning when the backend is unavailable: cached data is shown, actions requiring the backend are disabled, and a reconnect indicator is displayed. The UI never crashes due to backend absence.

## Security and permissions

### Capability-based policy enforcement (Phase 1)

Each module declares the permissions it requires in its Go `ModuleInfo`. Permissions are granted by the user via configuration (`config.toml`). These are two distinct concepts:

- **Declared permissions:** What a module says it needs to function. Defined in Go code as part of `ModuleInfo`.
- **Granted permissions:** What the user has allowed. Defined in `config.toml` under `[permissions]`.

A module can only use a capability if it both declares the permission **and** the user has granted it. Missing, malformed, or invalid permission grants **fail closed** — they must never accidentally grant access.

| Permission | Description | Risk |
|------------|-------------|------|
| `"system.info"` | Read CPU, RAM, disk, battery from /proc and sysfs | Low |
| `"window.manage"` | Query and control windows via Hyprland IPC | Medium |
| `"clipboard.read"` | Read clipboard contents | Medium |
| `"clipboard.write"` | Write to clipboard | Medium |
| `"audio.control"` | Playback routing, microphone controls, and audio metadata | Medium |
| `"screen.capture"` | Screenshot/screencast via xdg-desktop-portal | High |
| `"process.manage"` | List, signal, kill processes | High |
| `"filesystem.access"` | Read/write user files | High |
| `"network.access"` | Outgoing network connections | Medium |
| `"package.manage"` | Install/remove packages via pacman | Critical |

### Fault isolation vs security sandboxing

- **Fault isolation** (out-of-process services): prevents a module crash from taking down the entire application. Implemented in Phase 1.
- **Security sandboxing** (OS-level restrictions): NOT implemented in Phase 1. All modules run as the user and are subject to the same OS permissions as the user. Same-user processes are not isolated from each other's OS-level access. Actual OS-level sandboxing (systemd `RestrictNamespaces=`, `PrivateTmp=`, `ProtectSystem=`) is future work.

Omnibar never uses `sudo` or escalates privileges. All operations run as the invoking user.

## Data and configuration

### XDG directory layout

| Purpose | Path | Example |
|---------|------|---------|
| Configuration | `$XDG_CONFIG_HOME/omnibar/config.toml` | `~/.config/omnibar/config.toml` |
| Persistent data | `$XDG_DATA_HOME/omnibar/omnibar.db` | `~/.local/share/omnibar/omnibar.db` |
| Runtime sockets | `$XDG_RUNTIME_DIR/omnibar/omnibar.sock` | `/run/user/1000/omnibar/omnibar.sock` |
| Logs | `$XDG_STATE_HOME/omnibar/logs/` | `~/.local/state/omnibar/logs/` |
| Cache | `$XDG_CACHE_HOME/omnibar/` | `~/.cache/omnibar/` |

### Configuration

- Format: TOML
- Configuration validation is **application-defined** — TOML itself has no built-in schema feature. Omnibar parses TOML and then validates the parsed structure against its own expectations.
- Invalid sections fall back to defaults without discarding the whole config.
- `config_version` field supports additive migrations.
- Unknown modules produce warnings, not failures.

### Storage

- SQLite is kept as the selected future storage solution, but the SQLite driver and database implementation are **not introduced in Phase 1**.
- Explicit clipboard favorites use bounded versioned JSON (ADR-0016). SQLite is deferred until a feature needs querying or richer persistent data (e.g., monitoring history).
- WAL mode for concurrent reads.
- Schema version in a `meta` table; migrations are transactional and additive.
- Phase 1: no database tables required (settings live in TOML).

**SQLite-safe backup:** Copying only the main SQLite database file while WAL mode is active may omit committed data that exists only in the WAL file. A safe backup uses the SQLite online backup API, or performs a controlled checkpoint followed by shutdown, then copies the main database file.

## Hyprland integration

One component owns compositor state and event integration: the **Hyprland integration service** in the core. It:

- Reads `$HYPRLAND_INSTANCE_SIGNATURE` and `$XDG_RUNTIME_DIR/hypr/[HIS]/.socket.sock`
- Is the single owner of the Hyprland IPC socket connection
- Subscribes to Hyprland events and fans them out to interested modules
- Exposes a controlled API to modules; modules do not open their own Hyprland sockets

This avoids duplicate integrations and unnecessary communication paths.

If Hyprland is not detected (e.g., running outside Hyprland), window-management modules are disabled with a clear message; other modules continue working.

Omnibar does **not** modify `hyprland.conf`, Waybar config, Ghostty config, or replace existing Omarchy components.

## Performance and reliability targets

These are **initial targets requiring measurement**, not guarantees.

| Metric | Target |
|--------|--------|
| Idle CPU | < 1% |
| Idle RAM (backend) | < 20 MB |
| Idle RAM (UI) | < 60 MB |
| Startup time (UI visible) | < 500 ms |
| Startup time (backend ready) | < 200 ms |
| Crash recovery | < 5 s |

- Prefer event-driven operation over polling (Hyprland IPC events, PipeWire events)
- Bounded periodic sampling is acceptable where event-driven monitoring is unavailable
- Lazy module loading: only enabled modules are started
- No unnecessary background processes or animations

## Updates and distribution

- Distribution via Arch PKGBUILD and the Omarchy Package Repository (OPR)
- No self-updater (pacman/OPR manage updates)
- Install: `makepkg -si` from PKGBUILD, or `omarchy pkg add omnibar` via OPR (package installation command)
- Omarchy's documented system update flow is `omarchy update` (accounts for Omarchy's update guard and migration workflow)
- Upgrade individual packages via pacman (`pacman -Syu`)
- Rollback: downgrading a single package (`pacman -U <previous-package>.pkg.tar.zst` from the package cache) is distinct from rolling back an entire Omarchy system update using a system snapshot
- Uninstall: `pacman -Rs omnibar` (user data preserved in `$XDG_CONFIG_HOME`, `$XDG_DATA_HOME`, `$XDG_STATE_HOME`)
- Full data removal: manual deletion of XDG directories when explicitly requested
- SQLite migrations run automatically on first start after upgrade

OPR publication requires a package recipe contribution and acceptance by the OPR maintainers; creating a PKGBUILD alone does not publish the package.

## Directory structure

```
omnibar/
├── README.md
├── REQUIREMENTS.md
├── ARCHITECTURE.md
├── ROADMAP.md
├── CHANGELOG.md
├── DEVELOPMENT.md
├── docs/
│   ├── architecture/
│   │   ├── ADR-0001-project-architecture.md
│   │   ├── ADR-0002-ui-framework.md
│   │   ├── ADR-0003-ipc-protocol.md
│   │   ├── ADR-0004-module-execution-model.md
│   │   ├── ADR-0005-configuration-format.md
│   │   ├── ADR-0006-storage.md
│   │   ├── ADR-0007-permissions-security.md
│   │   ├── ADR-0008-packaging-updates.md
│   │   ├── ADR-0009-event-subscription.md
│   │   ├── ADR-0010-system-monitor.md
│   │   ├── ADR-0011-keep-awake.md
│   │   ├── ADR-0012-audio-mixer.md
│   │   ├── ADR-0013-stream-routing.md
│   │   ├── ADR-0014-microphone-controls.md
│   │   ├── ADR-0015-clipboard-history.md
│   │   └── ADR-0016-clipboard-favorites.md
│   ├── development/
│   └── features/
├── internal/
│   ├── core/
│   ├── ipc/
│   ├── modules/
│   └── services/
├── ui/
│   └── qml/
├── pkgbuild/
│   └── PKGBUILD
├── systemd/
│   └── omnibar.service
├── config/
│   └── config.toml.example
├── go.mod
├── go.sum
└── tests/
```

The initial structure is minimal. Directories are created only when code is added to them.

## Unresolved decisions

- Module permission UI: how users grant/revoke permissions (config file only in Phase 1; a settings UI may come later)
- Exact SQLite schema for future features (persistent clipboard history, monitoring history) — designed when those features are implemented
- Whether future external modules use separate executables with manifests (concept accepted, details TBD)
- OS-level sandboxing mechanism (future work)
