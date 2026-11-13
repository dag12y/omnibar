# Omnibar Implementation Milestone Plan

This document breaks the approved architecture into small, sequential, independently verifiable implementation milestones. It records both planned work and verified implementation progress. M1–M8 form Phase 1 (core foundation) and are complete. M9–M15 implement Phase 2 (system monitoring: CPU, RAM, swap, disk, network, battery) and conclude with the v0.1.0 release.

## Conventions

- Each milestone has a fixed ID (M1, M2, …) and must be completed, tested, and reviewed before the next begins.
- Milestones are deliberately small: each should be implementable, testable, reviewable, and committable independently.
- "Expected files" describes likely paths; exact implementation decisions are made during the milestone.
- Testing strategy distinguishes unit tests (Go `testing`), integration tests (backend+UI together), and manual verification steps.
- No SQLite, no dynamic Go plugin loading, no self-updater, and no OS sandboxing are introduced in this plan. M9–M15 add event delivery, system metrics monitoring (CPU, RAM, swap, disk, network, battery), and the first release; history, alerts, and other monitors are later work.
- Each milestone ends with a commit of the form `feat: … (Mn)`; planning changes are committed separately as `docs: …`.

## Milestone sequence overview

| ID | Title | Purpose | Status |
|----|-------|---------|--------|
| M1 | Go project scaffold & build system | Establish the Go module, entrypoint, and build tooling | Done |
| M2 | Logging & configuration loading | slog logging and TOML config with defaults and validation | Done |
| M3 | Unix domain socket server & IPC framing | Socket security, stale-socket handling, newline-delimited framing, graceful shutdown | Done |
| M4 | JSON-RPC 2.0 protocol & handshake | Request/response framing, handshake, error codes, protocol versioning | Done |
| M5 | Module registry (static registration) | ModuleInfo, registry, lifecycle states, panic recovery at module boundaries | Done |
| M6 | Quickshell QML UI & socket client | Minimal QML shell, Quickshell.Io.Socket connection, handshake, offline mode, reconnection | Done |
| M7 | End-to-end integration & verification | Backend + UI together, full handshake + request/response, graceful shutdown, integration tests | Done |
| M8 | Packaging & systemd user service | PKGBUILD, omnibar.service, config.toml.example installation, local makepkg verification | Done |
| M9 | Event subscription & delivery | Event hub, `module.subscribe`/`module.unsubscribe`, `module.event` push, slow-client policy, UI re-subscribe | Done |
| M10 | System monitor module (CPU, memory, swap) | First real module and capability consumer; on-demand sampling; `system.getSnapshot` and events | Done |
| M11 | UI for system monitoring | Live CPU/memory/swap cards, stale/offline/denied states, reusable components | Done |
| M12 | Disk monitoring (filesystem storage & I/O) | /proc/diskstats and Statfs, disk snapshot & events, UI disk card | Done |
| M13 | Network monitoring (interfaces & transfer rates) | /proc/net/dev parser, bandwidth deltas, network snapshot & events, UI network card | Done |
| M14 | Battery and power monitoring | sysfs power_supply reader, charge level, status, battery snapshot & events, UI battery card | Done |
| M15 | Release v0.1.0 | Tag, release, real checksums, install verification | Done |
| M16 | Panel usability and Keep Awake | Keyboard controls, bounded scrolling, visibility-aware monitoring, timed logind/Wayland inhibition | Done |
| M17 | Playback audio mixer | PipeWire compatibility adapter, volume/mute, default output, Audio page, isolated service-recovery checks | Done |
| M18 | Playback-stream output routing | Per-row device selector, live target validation, confirmed-state reconciliation, isolated two-stream checks | Done |
| M19 | Microphone controls and recording apps | Default input, gain/mute, capture metadata, Microphone view, isolated virtual-mic verification | Done |
| M20 | Session clipboard history | Explicit collection, bounded text history, search/copy/delete/clear, separate grants, nested Wayland verification | Done |
| M21 | Clipboard favorites | Explicit bounded persistence, protected favorites, filter, restore, and removal | Done |
| M22 | Session File shelf | Local references, URL drops/path input, search, open folder, copy path, and missing-item recovery | Done |
| M23 | Clipboard utilities | Transform & copy: trim edges, lowercase, uppercase, and line-break removal | Done |
| M24 | Running-window switcher | Bounded Hyprland windows, search, exact focus, workspace switching, and confirmation | Done |
| M25 | Application launcher | Installed desktop entries, search, explicit GIO launch, and Apps page | Done |
| M26 | Unified app/window search | Shared query, ranked results, keyboard selection, and explicit Switch/Launch | Done |

---

## M1 — Go project scaffold & build system

### Purpose
Establish the Go module, the backend entrypoint, and the build tooling so that subsequent milestones have a compilable foundation.

### Scope
- Initialize a Go module (`go.mod`) for the Omnibar backend.
- Create a minimal `main.go` that parses command-line flags (e.g., config path, log level) and exits cleanly.
- Add a build tool (Makefile or equivalent) for `build`, `test`, `run`, and `clean` targets.
- Add a trivial unit test confirming the package compiles and the entrypoint starts.

### Prerequisites
- Approved architecture (Go backend, static binary).
- Go 1.27 available locally.

### Implementation tasks
1. Create `go.mod` with module path `omnibar` and Go version directive.
2. Create `cmd/omnibar/main.go` with flag parsing and a clean exit path.
3. Create `Makefile` with `build`, `test`, `run`, `clean` targets.
4. Add `tests/` directory placeholder note (tests created in later milestones).
5. Write a unit test that builds the package and verifies the entrypoint starts without error.

### Expected files/packages
- `go.mod`
- `cmd/omnibar/main.go`
- `Makefile`
- `cmd/omnibar/main_test.go` (trivial compile/exit test)

### Testing strategy
- `go build ./...` succeeds.
- `go test ./...` passes.
- Running the built binary exits with code 0 and prints no fatal errors.

### Acceptance criteria
- The Go backend binary builds from `go.mod` with no external dependencies beyond the standard library.
- `go test ./...` passes.
- The binary accepts `--help` or equivalent flag display and exits cleanly.

### Definition of done
- Code committed with a `feat:` or `chore:` message.
- `go build ./...` and `go test ./...` both pass.
- No uncommitted changes remain.

### Risks and dependencies
- Go module path naming must be decided (recommend `omnibar` or `github.com/<user>/omnibar`).
- Build tool choice (Make vs. just a shell script) is minor but should be consistent with Omarchy app patterns (`hype` uses `bin/build`).

---

## M2 — Logging & configuration loading

### Purpose
Add structured logging (slog) and TOML configuration loading with defaults, validation, and fail-closed permission semantics, so the backend can be configured and observed.

### Scope
- Initialize `log/slog` with configurable log level.
- Parse TOML configuration from `$XDG_CONFIG_HOME/omnibar/config.toml`.
- Define application defaults for missing sections.
- Validate module names, permission names, and value types (application-defined, not TOML schema).
- Fail-closed permission evaluation: missing/malformed grants deny access.
- Create `config.toml.example` for reference.

### Prerequisites
- M1 complete (Go module and entrypoint exist).

### Implementation tasks
1. Add a TOML parsing dependency (e.g., `BurntSushi/toml` or `pelletier/go-toml`) — evaluated but not yet added in architecture; decision recorded during M2.
2. Implement `log/slog` initialization with level from config or default `info`.
3. Define configuration structs: `CoreConfig`, `ModuleConfig`, `Permissions`.
4. Implement config loader: read file, parse TOML, apply defaults, validate structure.
5. Implement fail-closed permission check: missing `[permissions]` table → no permissions granted.
6. Create `config/config.toml.example` with commented example.
7. Write unit tests for config loading, defaults, validation, and fail-closed behavior.

### Expected files/packages
- `internal/config/` (config types, loader, validator)
- `internal/logging/` (slog initialization) or inline in `internal/core/`
- `config/config.toml.example`
- `cmd/omnibar/main.go` updated to load config

### Testing strategy
- Unit tests for config parsing (valid, invalid, missing file).
- Unit tests for defaults applied when sections are absent.
- Unit tests for fail-closed permissions (missing table, malformed key, unknown module).
- Manual: run backend with a sample `config.toml` and verify log output.

### Acceptance criteria
- Backend starts with no config file and uses defaults.
- Backend starts with a valid `config.toml` and applies values.
- Invalid permission grants result in access denied (fail closed), never accidental grant.
- `go test ./...` passes.

### Definition of done
- Config loading and validation covered by unit tests.
- Logging outputs structured records at the configured level.
- Code committed; `go build ./...` and `go test ./...` pass.

### Risks and dependencies
- TOML library choice (third-party dependency) must be recorded and justified.
- `config_version` field semantics (additive migrations only) must be implemented consistently.

---

## M3 — Unix domain socket server & IPC framing

### Purpose
Create the Go-owned Unix domain socket server with secure socket permissions, stale-socket handling, newline-delimited JSON framing, and graceful shutdown.

### Scope
- Create socket directory `$XDG_RUNTIME_DIR/omnibar/` with mode `0700`.
- Bind socket file `$XDG_RUNTIME_DIR/omnibar/omnibar.sock` with mode `0600`.
- Fatal error if `XDG_RUNTIME_DIR` is unset (no fallback to insecure locations).
- Stale socket detection: if a socket file exists, attempt to connect; if live backend responds, exit with error; if connection fails, remove only the confirmed-socket file and bind.
- Newline-delimited framing: read until `\n`, parse one JSON object, enforce 1 MiB max message size.
- Graceful shutdown on `SIGTERM`/`SIGINT`: stop accepting connections, close clients, stop modules, remove socket file, exit 0.

### Prerequisites
- M1 and M2 complete (Go module, config loading, logging).

### Implementation tasks
1. Implement socket directory creation with mode `0700`.
2. Implement socket file binding with mode `0600`.
3. Implement stale-socket detection and safe removal (confirm file is a socket before removing).
4. Implement newline-delimited frame reader/writer with 1 MiB max message size.
5. Implement signal handling for `SIGTERM` and `SIGINT`.
6. Implement graceful shutdown sequence.
7. Write unit tests for framing (valid message, oversized message, incomplete frame) and stale-socket handling.

### Expected files/packages
- `internal/ipc/` (socket server, framing, connection handling)
- `cmd/omnibar/main.go` updated to start socket server

### Testing strategy
- Unit tests for frame reading/writing (newline termination, max size rejection).
- Unit tests for stale-socket detection (live socket vs. stale file).
- Integration-lite test: bind socket, connect, send one framed message, receive echo.
- Manual: verify socket file permissions (`ls -l` shows 0600, directory 0700).
- Manual: send `SIGTERM`, verify socket file removed and process exits 0.

### Acceptance criteria
- Socket directory created with mode `0700`.
- Socket file created with mode `0600`.
- Backend exits with fatal error if `XDG_RUNTIME_DIR` unset.
- Stale socket file removed only after confirming it is a socket.
- Framed messages are newline-delimited and bounded at 1 MiB.
- `SIGTERM` triggers graceful shutdown (socket removed, exit 0).
- `go test ./...` passes.

### Definition of done
- Socket server and framing covered by unit tests.
- Socket permissions verified manually.
- Graceful shutdown verified manually.
- Code committed; `go build ./...` and `go test ./...` pass.

### Risks and dependencies
- `XDG_RUNTIME_DIR` may be unset in some environments (non-graphical sessions); fatal error path must be tested.
- Stale-socket race condition: two backends starting simultaneously; connect-then-exit logic must be atomic enough to avoid conflicts.

---

## M4 — JSON-RPC 2.0 protocol & handshake

### Purpose
Implement the JSON-RPC 2.0 protocol layer on top of the socket framing: request parsing, response construction, handshake, error codes, and protocol versioning.

### Scope
- Parse JSON-RPC 2.0 requests: `jsonrpc`, `id`, `method`, `params`.
- Construct responses: same `id`, either `result` or `error`, never `method`.
- Handle notifications (no `id`, no response expected).
- Implement `client.handshake` (UI → backend) and `server.handshake` (backend → UI).
- Protocol version `omnibar.ipc/v1`; mismatch rejected with error -32600.
- Standard error codes -32700 to -32603; custom server errors -32000 to -32999.
- Backend errors include optional `module` field for routing.

### Prerequisites
- M3 complete (socket server and framing exist).

### Implementation tasks
1. Define JSON-RPC request/response/notification structures.
2. Implement request parser and response builder.
3. Implement `client.handshake` handler (validate protocol version, return available modules).
4. Implement `server.handshake` response structure.
5. Implement error code mapping (standard + custom ranges).
6. Implement protocol version mismatch rejection.
7. Write unit tests for request parsing, response construction, handshake, and error codes.

### Expected files/packages
- `internal/ipc/jsonrpc/` (protocol types, handler, error codes)
- `internal/ipc/handshake.go` (handshake logic)

### Testing strategy
- Unit tests for valid/invalid JSON-RPC requests.
- Unit tests for response construction (same id, no method field).
- Unit tests for handshake success and version mismatch.
- Unit tests for standard and custom error codes.
- Manual: use `socat` or `nc` to send a handshake request to the socket and verify response.

### Acceptance criteria
- Every request has an ID; response uses the same ID.
- Responses never contain a `method` field.
- Handshake succeeds with matching protocol version.
- Handshake fails with error -32600 on version mismatch.
- Notifications have no ID and expect no response.
- `go test ./...` passes.

### Definition of done
- JSON-RPC protocol covered by unit tests.
- Handshake verified manually with `socat`/`nc`.
- Code committed; `go build ./...` and `go test ./...` pass.

### Risks and dependencies
- JSON-RPC 2.0 compliance must be exact (no `method` in responses, correct ID matching).
- Protocol version string must match between ADR-0003 and implementation.

---

## M5 — Module registry (static registration)

### Purpose
Implement the module registry with static registration of built-in Go modules, lifecycle states, and panic recovery at module execution boundaries.

### Scope
- Define `ModuleInfo` struct (ID, name, version, category, permissions, dependencies, factory).
- Implement `ModuleRegistry`: register, list, get, initialize, start, stop, dispose.
- Module lifecycle states: Registered → Initialized → Started → Running → Stopped → Disposed.
- Panic recovery at controlled module execution boundaries (recover only in same goroutine).
- Mark failed modules, log error and stack trace, stop dependent functionality safely.
- Only restart modules that explicitly support safe reinitialization.
- No dynamic Go plugin loading.

### Prerequisites
- M1 through M4 complete (Go module, config, logging, socket, JSON-RPC).

### Implementation tasks
1. Define `ModuleInfo` and `Module` interface.
2. Implement `ModuleRegistry` with static registration.
3. Implement lifecycle transitions (initialize, start, stop, dispose).
4. Implement panic recovery wrapper for module execution boundaries.
5. Implement failed-module marking and safe stop of dependents.
6. Register a placeholder built-in module (e.g., a "core" or "system" module that exposes status via JSON-RPC).
7. Write unit tests for registration, lifecycle, panic recovery, and fail-closed permission checks.

### Expected files/packages
- `internal/modules/` (registry, ModuleInfo, Module interface)
- `internal/modules/core/` (placeholder built-in module)

### Testing strategy
- Unit tests for module registration and lookup.
- Unit tests for lifecycle transitions.
- Unit tests for panic recovery at module boundary (recover in same goroutine only).
- Unit tests for permission check integration (module declares permission, user grants/denies).
- Manual: verify a registered module can be listed and queried via JSON-RPC.

### Acceptance criteria
- Modules register statically at compile time.
- Lifecycle states transition correctly.
- Panic at module boundary is recovered; module marked failed; dependents stopped safely.
- Modules cannot use capabilities without both declared and granted permissions.
- No dynamic package loading.
- `go test ./...` passes.

### Definition of done
- Module registry covered by unit tests.
- Placeholder module responds to JSON-RPC queries.
- Code committed; `go build ./...` and `go test ./...` pass.

### Risks and dependencies
- `recover()` cannot catch segfaults; this limitation must be documented and not overstated.
- Inter-module communication goes through the core (hub-and-spoke), not direct references.

---

## M6 — Quickshell QML UI & socket client

### Purpose
Create the Quickshell QML UI that connects to the Go backend via `Quickshell.Io.Socket`, performs the handshake, and displays connection status with offline mode and reconnection.

### Scope
- Minimal QML shell.qml using `Quickshell.Io.Socket`.
- Connect to `$XDG_RUNTIME_DIR/omnibar/omnibar.sock`.
- Send `client.handshake` on connect.
- Display "Connected", "Connecting…", or "Backend unavailable" status.
- Reconnection with exponential backoff (1s, 2s, 4s, 8s, 16s, then capped at 30s).
- Offline mode: UI remains responsive, backend-dependent actions disabled.
- UI never crashes when backend is absent.

### Prerequisites
- M1 through M5 complete (backend socket server, JSON-RPC, module registry).
- Quickshell 0.3.2 installed locally (verified during research).

### Implementation tasks
1. Create `ui/qml/shell.qml` with a minimal Quickshell shell.
2. Import `Quickshell.Io` and use `Socket` type to connect to the backend socket.
3. Implement handshake send on connect.
4. Implement connection status display (connected/connecting/offline).
5. Implement reconnection timer with exponential backoff.
6. Implement graceful degradation: UI usable without backend.
7. Write unit tests for QML logic where possible (Qt Test), otherwise manual verification.

### Expected files/packages
- `ui/qml/shell.qml`
- `ui/qml/components/` (connection status indicator, etc.)

### Testing strategy
- Manual: launch backend, launch UI, verify connection status shows "Connected".
- Manual: stop backend, verify UI shows "Backend unavailable" and does not crash.
- Manual: restart backend, verify UI reconnects with backoff.
- Qt Test for QML logic (if applicable to the shell structure).

### Acceptance criteria
- UI launches via `quickshell` binary (verified CLI options: `-c` or `-p`).
- UI connects to backend socket and performs handshake.
- UI displays connection status accurately.
- UI remains responsive when backend is unavailable.
- Reconnection attempts occur with exponential backoff.

### Definition of done
- UI connects to backend and displays status.
- UI degrades gracefully without backend.
- Code committed; no build errors.

### Risks and dependencies
- Quickshell QML API may change between versions; pin to 0.3.2 and test compatibility.
- Exact `quickshell` launch command for Omnibar config (`-c omnibar` or `-p <path>`) determined at packaging time; M6 uses a path-based launch for development.
- QML socket client must handle asynchronous connection events correctly.

---

## M7 — End-to-end integration & verification

### Purpose
Verify the complete Phase 1 foundation works end-to-end: backend + UI together, full handshake, request/response, graceful shutdown, and integration-level tests.

### Scope
- Run backend and UI together as separate processes.
- Verify full connection lifecycle: UI connect → handshake → request/response, plus no-response behavior for JSON-RPC notifications.
- Verify backend restart: UI reconnects and re-handshakes.
- **Deferred:** `module.subscribe` and backend-pushed `module.event` notifications. No Phase 1 milestone defines event payloads or an event-producing module, so subscription is implemented together with the first such module (Phase 2). Re-subscribe on reconnect moves with it.
- Verify graceful shutdown: socket removed, exit 0.
- Add integration tests that spawn the backend and a simulated UI client and assert protocol-level behavior (`tests/integration/`).
- Add `scripts/e2e-ui.sh` (`make e2e-ui`), which runs the real Quickshell UI against the real backend and asserts connect, handshake, offline backoff, and reconnect.
- Verify fail-closed permissions end-to-end. The shipped `core` module declares no capabilities, so the test uses a test-only module and method that call the production `Registry.HasCapability`; a production capability-consuming method is deferred to the first module that needs one.

### Prerequisites
- M1 through M6 complete.

### Implementation tasks
1. Create integration test harness (Go test that spawns backend process, connects as a client, performs handshake, sends request, verifies response).
2. Verify backend restart scenario (kill backend, UI reconnects).
3. Verify graceful shutdown (SIGTERM, socket removed).
4. Verify fail-closed: request for capability without permission returns error.
5. Write documentation in `docs/development/` for local run workflow (how to start backend and UI separately for testing).

### Expected files/packages
- `tests/integration/` (integration test harness)
- `docs/development/local-development.md` (run workflow)

### Testing strategy
- Integration tests: spawn backend, connect via socket, assert handshake success, request/response round-trip, error responses.
- Manual end-to-end: `./build/omnibar` backend + `quickshell -p ui/qml/shell.qml` UI, verify connection UI shows "Connected".
- Manual: kill backend, observe UI offline mode; restart backend, observe reconnection.

### Acceptance criteria
- Backend and UI communicate over Unix socket with JSON-RPC 2.0.
- Handshake completes with matching IDs and protocol version.
- Request/response round-trip works (same ID, no `method` in response).
- Backend restart triggers UI reconnection with backoff (verified by `make e2e-ui`).
- Graceful shutdown removes socket and exits 0.
- Fail-closed permissions enforced end-to-end.
- `go test ./...` passes (unit + integration).

### Definition of done
- Full end-to-end flow verified manually and via integration tests.
- Local development workflow documented.
- Code committed; `go build ./...` and `go test ./...` pass.

### Risks and dependencies
- Running two processes in tests requires process management and cleanup.
- Timing-sensitive reconnection tests may be flaky; use timeouts and retries.

---

## M8 — Packaging & systemd user service

### Purpose
Create the Arch PKGBUILD, systemd user service unit, and config.toml.example installation so Omnibar can be installed and managed like other Omarchy Qt apps (`hype`, `monologue`, `omacut`).

### Scope
- Create `pkgbuild/PKGBUILD` following Omarchy app packaging patterns.
- Create `systemd/omnibar.service` user unit (not `@.service`, one backend per user).
- Install `config/config.toml.example` to the package.
- Verify local `makepkg` build produces a valid package.
- Document that OPR publication requires maintainer acceptance (not part of this milestone).
- No self-updater.
- **Decisions:** the UI is installed to `/usr/share/omnibar/qml/` and launched with `quickshell -n -p` via an `omnibar-ui` wrapper (not `-c`, so `XDG_CONFIG_DIRS` and user configs cannot shadow it). The package is MIT-licensed and its `pkgver` must equal `ipc.ServerVersion` (enforced by `tests/packaging`). Only the backend gets a systemd unit; the UI is not autostarted. `make package` builds from the working tree; `makepkg` against the GitHub tag tarball happens at release time. See `docs/development/packaging.md`.

### Prerequisites
- M1 through M7 complete (working backend + UI).

### Implementation tasks
1. Create `pkgbuild/PKGBUILD` with dependencies (Go, Quickshell, Qt6).
2. Create `systemd/omnibar.service` user unit.
3. Package `config.toml.example` and QML files.
4. Verify `makepkg` builds successfully in a clean chroot or local environment.
5. Document install/upgrade/rollback/uninstall flow (pacman-based, no self-updater).
6. Document that OPR publication is a separate step requiring OPR maintainer acceptance.

### Expected files/packages
- `pkgbuild/PKGBUILD`
- `systemd/omnibar.service`
- Packaging notes in `docs/development/` or `README.md`

### Testing strategy
- `makepkg -si` builds and installs without errors.
- Installed binary runs and creates socket at correct path.
- `systemctl --user enable --now omnibar` starts the backend.
- Uninstall (`pacman -Rs`) preserves user data in XDG directories.

### Acceptance criteria
- Package builds with `makepkg`.
- systemd user service starts the backend.
- Installed UI launches via `quickshell` and connects to backend.
- No self-updater code exists.
- User data preserved across install/uninstall.

### Definition of done
- PKGBUILD and systemd unit committed.
- Local package install verified.
- Code committed; `go build ./...` and `go test ./...` pass.

### Risks and dependencies
- PKGBUILD dependencies must match actual import paths (decided in M1/M2).
- OPR publication is not part of this milestone; creating a PKGBUILD alone does not publish.

---

## M9 — Event subscription & delivery

### Purpose
Let the backend push `module.event` notifications to subscribed clients, and let the UI subscribe and re-subscribe, so Phase 2 monitors can stream live data. Semantics are fixed in ADR-0009.

### Scope
- `internal/events`: in-process event hub (publish, subscribe, per-module subscriber tracking).
- Per-connection session in `internal/ipc`: subscription set, bounded outbound queue (default 64), single writer goroutine per connection.
- Handlers receive the calling session through the request context. Existing `ipc.New`, `Dispatcher.Register`, and handler call sites keep working.
- `module.subscribe` and `module.unsubscribe` with the params, results, and errors from ADR-0009 (-32602, -32600, -32001, -32002).
- `module.event` notifications: `{module, event, data}`, no `id`.
- Slow-client policy: never block `Publish`; disconnect a client whose queue is full.
- UI: subscribe to every module from the handshake result, track per-module subscription state, handle `module.event`, and re-subscribe after every successful handshake.
- Protocol version stays `omnibar.ipc/v1` and backend version stays `0.1.0` (the first release is M12).
- Out of scope: any real event producer (M10), event history or replay, persistent subscriptions.

### Prerequisites
- M1 through M8 complete.
- ADR-0009 accepted.

### Implementation tasks
1. Add `internal/events` with `Hub`: `Publish`, `Subscribe`/unsubscribe handles, `HasSubscribers(module)`, and a change callback for none↔some transitions. Unit tests including `-race`.
2. Add the IPC session: one writer goroutine per connection fed by a bounded queue; route replies and notifications through it; drop the client on overflow.
3. Pass the session to handlers via context; add context-aware registration to the dispatcher without changing existing signatures.
4. Implement `module.subscribe` / `module.unsubscribe` with capability and module-state checks.
5. Add a way for modules to publish through the hub as a running module only (publishing from a stopped module is a no-op logged at debug).
6. UI: add a small pure-JS protocol helper (build requests, classify responses vs notifications, plan subscriptions) with Qt Test coverage; wire `BackendClient.qml` to subscribe after handshake and expose a `moduleEvent` signal and subscription state.
7. Extend integration tests with an in-process backend and a test-only publisher module.
8. Extend `scripts/e2e-ui.sh` so the UI logs a subscription after the first handshake and again after the backend restart.
9. Update docs: `local-development.md`, `ARCHITECTURE.md` if the implementation deviates, `CHANGELOG.md`.

### Expected files/packages
- `internal/events/` (hub and tests)
- `internal/ipc/session.go`, `internal/ipc/subscribe.go` (names indicative) and tests
- `internal/ipc/jsonrpc/` (context-aware dispatch)
- `ui/qml/components/protocol.js`, `ui/qml/components/BackendClient.qml`, `ui/qml/tests/tst_protocol.qml`
- `tests/integration/events_test.go`

### Testing strategy
- Unit: hub publish/subscribe/unsubscribe, filtering, subscriber counts, panic isolation, concurrency under `-race`.
- Unit: session queue overflow disconnects only the slow client; frames never interleave.
- Integration (real socket): subscribe, receive events in order, event filter, unsubscribe stops delivery, multiple clients, disconnect cleans up, permission denied, unknown module, module not running, invalid params, idempotent subscribe/unsubscribe.
- Integration: a client that never reads does not delay a fast client or the publisher; graceful shutdown with active subscriptions exits 0 with no errors logged.
- QML: Qt Test for the protocol helper.
- End to end: `make e2e-ui` shows subscribe, restart, then re-subscribe in the UI log.
- Real events from a real module are verified end to end in M10/M11, not here; the shipped `core` module publishes none.

### Acceptance criteria
- Subscription semantics match ADR-0009 exactly, including error codes.
- Events reach only subscribed clients, filtered and in per-module order.
- A slow client is disconnected without blocking the publisher or other clients.
- Subscriptions end when the connection closes (no leaked goroutines or entries).
- The UI re-subscribes after each reconnect.
- Fail-closed: a module with ungranted capabilities cannot be subscribed to.
- `go test -race ./...` and `make ui-test` pass.

### Definition of done
- Acceptance criteria met and covered by tests.
- ADR-0009 and any deviations documented.
- Code committed; `go build ./...`, `go vet ./...`, and `go test ./...` pass; `make e2e-ui` passes.

### Risks and dependencies
- Adding a writer goroutine touches the connection code that M3 hardened (shutdown races, ownership-verified cleanup). The existing server tests must keep passing unchanged.
- Context plumbing must not change behavior for existing handlers.
- Overflow handling must not deadlock against shutdown.

---

## M10 — System monitor module (CPU, memory, swap)

### Purpose
Add the first real module: read-only CPU, memory, and swap monitoring, published as events and available as a snapshot. It is also the first production consumer of a capability, replacing the test-only stand-in from M7. Design is fixed in ADR-0010.

### Scope
- Built-in `system_monitor` module declaring `system.info`.
- Parsers for `/proc/stat` (total and per-core CPU) and `/proc/meminfo` (memory, swap), testable against fixture files.
- `system.getSnapshot` request; `cpu` and `memory` events through the hub.
- On-demand sampler: runs only while the module is running and has subscribers; configurable `interval_ms` (default 2000, 500-60000).
- Module-specific configuration validated at load; `modules.<name>.enabled` unchanged.
- Fail-closed: without the `system.info` grant, `system.getSnapshot` and subscriptions are denied.
- Update `config/config.toml.example`.
- Out of scope: history, alerts, per-process data, GPU, disk, network, battery.

### Prerequisites
- M9 complete.
- ADR-0010 accepted.

### Implementation tasks
1. Add parsers with table-driven tests: normal, missing fields, no swap, counter wrap or reset, malformed lines.
2. Add the module, its lifecycle, and the sampler goroutine with clean stop.
3. Hook the sampler to the hub's subscriber transitions.
4. Add `system.getSnapshot` and the `cpu`/`memory` events with permission checks.
5. Extend configuration with `interval_ms` and validation.
6. Register the module and update `config.toml.example`.
7. Integration tests over the real binary: snapshot, subscription events, denial without grant, disabled module, interval validation failing startup.
8. Replace the test-only capability method in the fail-closed integration test with `system.getSnapshot`; keep the other denial cases.
9. Measure idle backend CPU and RAM with no subscribers and with one, and record the result in `local-development.md`.

### Expected files/packages
- `internal/modules/systemmonitor/` (module, parsers, sampler, tests, fixtures)
- `internal/config/` (module settings and validation)
- `tests/integration/systemmonitor_test.go`
- `config/config.toml.example`

### Testing strategy
- Unit: parsers against fixtures; CPU percentage math, including a zero-delta interval; sampler start/stop and interval handling with a fake clock or injected sampler; no goroutine leak.
- Unit: configuration validation for `interval_ms`.
- Integration (real binary, real `/proc`): `system.getSnapshot` returns plausible values (percentages 0-100, `used <= total`); a subscriber receives events at roughly the configured interval; a second subscriber does not double the sampling rate; denial without a grant.
- Manual: with no UI connected, backend CPU stays near zero.

### Acceptance criteria
- Snapshot and events carry correct CPU, memory, and swap values.
- Sampling happens only while subscribed, and stops afterwards.
- Requests and subscriptions are denied unless `system.info` is granted.
- Invalid `interval_ms` fails configuration validation.
- Idle backend stays within the architecture targets (< 1% CPU, < 20 MB RAM) with no subscribers.
- `go test -race ./...` passes.

### Definition of done
- Acceptance criteria met and covered by tests.
- Idle measurements recorded.
- Code committed; `go build ./...`, `go vet ./...`, and `go test ./...` pass.

### Risks and dependencies
- `/proc/stat` wrap and reset cases and very short intervals can produce odd percentages; clamp to 0-100 and test.
- Container or unusual kernels may lack fields; parsers must return a clear error, not panic.
- Per-core arrays make event size depend on core count; stay well under the 1 MiB frame limit and test with a large synthetic core count.

---

## M11 — UI for system monitoring

### Purpose
Show live CPU, memory, and swap in the Quickshell UI, behave sensibly while offline or denied, and give the shell a layout that later modules can join.

### Scope
- Replace the connection-demo panel content with module cards: connection status header, a `system_monitor` card with CPU, memory, and swap readings, and the module list.
- Subscribe on connect (M9), fetch `system.getSnapshot` after subscribing, then apply `cpu` and `memory` events.
- Offline: keep the last values, mark them stale, disable backend-dependent actions.
- Denied (-32001) or unavailable (-32002): show a clear message on the card, no crash, no retry storm.
- Reusable components (metric bar, module card) and a pure-JS formatting helper with Qt Test coverage (bytes, percentages).
- Out of scope: history graphs, alerts, theme integration, settings UI.

### Prerequisites
- M9 and M10 complete.

### Implementation tasks
1. Split `shell.qml` into components under `ui/qml/components/` (module card, metric bar).
2. Add a formatting helper (`format.js`) with unit tests.
3. Wire the card to snapshot and events with stale/offline/denied states.
4. Extend `scripts/e2e-ui.sh` to assert that live values arrive and stop arriving as stale values after the backend stops.
5. Update the packaging file list if components are added; `tests/packaging` must still pass.
6. Manual check on the real session: values change under load, restart behavior, denied state with the permission turned off.

### Expected files/packages
- `ui/qml/shell.qml`, `ui/qml/components/*`, `ui/qml/components/format.js`
- `ui/qml/tests/tst_format.qml`
- `scripts/e2e-ui.sh`

### Testing strategy
- Qt Test for formatting and state logic.
- `make e2e-ui` for live values, stale after backend stop, and recovery after restart.
- Manual: screenshot and visual check; run a CPU load and watch the card; revoke `system.info` and confirm the denied message.

### Acceptance criteria
- CPU, memory, and swap update live while connected.
- Offline shows stale values marked as stale; reconnect resumes updates without user action.
- Denied and unavailable states are explained on the card.
- UI never crashes with the backend absent, restarting, or denying access.
- `make ui-test`, `make e2e-ui`, and `go test ./...` pass.

### Definition of done
- Acceptance criteria met.
- Packaging still installs every shipped QML file.
- Code committed.

### Risks and dependencies
- A layout that suits one card must still hold more later; keep components generic.
- Updating the UI at event rate must not cause visible jank or idle CPU in Quickshell; check with one subscriber at the default interval.

---

## M12 — Disk monitoring (filesystem storage & I/O)

### Purpose
Add disk storage usage and disk I/O throughput monitoring to `system_monitor`, extending the backend sampler, RPC snapshot, and Quickshell UI.

### Scope
- Parse `/proc/diskstats` in `internal/modules/systemmonitor`: compute read and write throughput (bytes/second) across physical block devices.
- Query root filesystem storage via `syscall.Statfs` (total, available, used bytes).
- Extend `system.getSnapshot` with the `disk` snapshot: storage bytes and I/O rates.
- Publish `disk` events (`module: "system_monitor"`, `event: "disk"`) through the event hub during sampling.
- Gated by the existing `system.info` capability.
- Quickshell UI: update `BackendClient.qml` and `shell.qml` to display disk storage and I/O throughput bars.
- Unit tests with `/proc/diskstats` fixtures and delta calculations.

### Implementation tasks
1. Add `internal/modules/systemmonitor/proc_diskstats.go` and table-driven tests with fixtures.
2. Implement root filesystem storage sampler using `syscall.Statfs`.
3. Integrate disk metrics into `computeSnapshot` and `sampler.go`.
4. Update `BackendClient.qml` to merge `disk` events and expose disk data.
5. Add disk metric representation in `shell.qml`.
6. Add unit and integration tests for disk snapshot and event delivery.

### Expected files/packages
- `internal/modules/systemmonitor/proc_diskstats.go`
- `internal/modules/systemmonitor/proc_diskstats_test.go`
- `internal/modules/systemmonitor/fixtures/diskstats_*`
- `ui/qml/shell.qml`
- `ui/qml/components/BackendClient.qml`

### Acceptance criteria
- Disk total, available, used bytes, and I/O rates update live in the UI.
- Idle disks report 0 B/s read and write without division errors or negative rates.
- Gated by `system.info` permission (denied without grant).
- `go test -race ./...`, `make ui-test`, and `make e2e-ui` all pass.

---

## M13 — Network monitoring (interfaces & transfer rates)

### Purpose
Add network bandwidth and interface transfer rate monitoring to `system_monitor`.

### Scope
- Parse `/proc/net/dev` in `internal/modules/systemmonitor`: calculate RX and TX byte transfer rates (bytes/second) across non-loopback interfaces.
- Extend `system.getSnapshot` with the `network` snapshot: per-interface and total RX/TX rates.
- Publish `network` events (`module: "system_monitor"`, `event: "network"`) through the event hub during sampling.
- UI: display live download and upload transfer rates in `shell.qml`.
- Unit tests with `/proc/net/dev` fixtures.

### Expected files/packages
- `internal/modules/systemmonitor/proc_netdev.go`
- `internal/modules/systemmonitor/proc_netdev_test.go`
- `internal/modules/systemmonitor/fixtures/netdev_*`
- `ui/qml/shell.qml`
- `ui/qml/components/BackendClient.qml`

### Acceptance criteria
- Live RX/TX rates update in the UI.
- Loopback interface (`lo`) is excluded from total network bandwidth.
- `go test -race ./...`, `make ui-test`, and `make e2e-ui` pass.

---

## M14 — Battery and power monitoring

### Purpose
Add battery level, charging status, and power draw monitoring to `system_monitor`.

### Scope
- Read `/sys/class/power_supply` in `internal/modules/systemmonitor`: detect batteries (`BAT*`) and AC line status.
- Gracefully handle desktop systems without batteries (report `present: false`).
- Capture capacity percentage, charge/discharge status, and power consumption rate in watts.
- Extend `system.getSnapshot` with `battery` payload and publish `battery` events.
- UI: battery card/badge in `shell.qml` with charging state indicator.
- Unit tests using a mock filesystem root.

### Expected files/packages
- `internal/modules/systemmonitor/sysfs_battery.go`
- `internal/modules/systemmonitor/sysfs_battery_test.go`
- `ui/qml/shell.qml`
- `ui/qml/components/BackendClient.qml`

### Acceptance criteria
- Laptop systems display battery charge percent, state, and power rate; desktops cleanly hide or report absent battery without error.
- All tests pass.

---

## M15 — Release v0.1.0

### Purpose
Turn the work so far into a tagged, installable release and complete the package verification that M8 could not do without installing.

### Scope
- Version `0.1.0` in `PKGBUILD` and the backend (already equal; `tests/packaging` enforces it).
- Update `CHANGELOG.md` for the release; tick completed Phase 2 items in `ROADMAP.md`.
- Tag `v0.1.0`, push, and create a GitHub release.
- Set real checksums with `updpkgsums` and verify a clean build from the GitHub tag tarball.
- Install and exercise the real package on the machine.
- Publishing to OPR remains separate and requires maintainer acceptance.
- No self-updater.

### Prerequisites
- M1 through M14 complete; all tests green.
- A public GitHub repository exists.

### Implementation tasks
1. Final pass: `go build ./...`, `go vet ./...`, `go test -race ./...`, `make ui-test`, `make e2e-ui`, `make package`.
2. Update `CHANGELOG.md` (move Unreleased to 0.1.0) and `ROADMAP.md`.
3. Commit, tag `v0.1.0`, push, create the GitHub release.
4. In `pkgbuild/`, run `updpkgsums`, then `makepkg -f` against the tag tarball; commit the checksum update.
5. Install with `sudo pacman -U`, enable the service, run `omnibar-ui`, and check connect, live values, restart, and uninstall (`pacman -Rs`) with user data preserved.
6. Record the verification in `docs/development/packaging.md` and note anything learned.

### Expected files/packages
- `CHANGELOG.md`, `ROADMAP.md`, `pkgbuild/PKGBUILD`
- Git tag `v0.1.0` and a GitHub release

### Testing strategy
- Full automated suite plus the manual install checklist in step 5.
- Reinstall over the top of the same version, and downgrade with `pacman -U`, to confirm rollback.

### Acceptance criteria
- `makepkg` builds from the published tag tarball with real checksums.
- The installed package runs the backend as a user service and the UI connects and shows live data.
- Uninstalling keeps `$XDG_CONFIG_HOME/omnibar`, `$XDG_DATA_HOME/omnibar`, and `$XDG_STATE_HOME/omnibar`.
- The tag, release notes, and changelog agree.

### Definition of done
- Release tagged and pushed; checksum commit pushed.
- Install verification documented.

### Risks and dependencies
- The `sudo pacman -U` step needs the user's password and cannot be automated by the agent.
- The GitHub tarball includes everything tracked at the tag, so anything the build needs must be committed (no ignored files).
- OPR acceptance is outside this milestone and outside the repository.

---

## Dependency graph

```
M1 → M2 → M3 → M4 → M5 → M6 → M7 → M8 → M9 → M10 → M11 → M12 → M13 → M14 → M15
```

Each milestone depends on all prior milestones. No milestone can begin until the previous one is complete, tested, reviewed, and committed.

---

## Development workflow (to be added to DEVELOPMENT.md)

A repeatable local development workflow:

1. `git checkout -b <feature-branch>` from `main`.
2. Implement changes for the current milestone.
3. `go build ./...` — verify compilation.
4. `go test ./...` — verify unit and integration tests.
5. Run backend manually: `./build/omnibar` (or `go run ./cmd/omnibar`).
6. Run UI manually: `quickshell -p ui/qml/shell.qml`.
7. Verify connection status in UI.
8. Commit with `feat:`, `fix:`, `test:`, `docs:`, or `chore:` prefix.
9. `git checkout main` and merge after review.

### Backend and UI separation
- Backend is a standalone Go binary; UI is launched separately via `quickshell`.
- Backend stays alive when UI closes; UI reconnects if backend restarts.
- For development, run both in separate terminals.

### Testing locally
- Unit tests: `go test ./...`
- Integration: `go test ./tests/integration/...`
- Manual end-to-end: start backend, start UI, verify "Connected" status.

---

## Remaining risks and open questions

| Risk | Mitigation |
|------|------------|
| Quickshell QML API may change between versions | Pin to 0.3.2; test compatibility before upgrades |
| TOML library choice not yet finalized | Decide during M2; document justification |
| `recover()` cannot catch segfaults | Document limitation; do not overstate crash recovery |
| `XDG_RUNTIME_DIR` may be unset | Fatal error with clear message; no insecure fallback |
| Stale socket race between two backend instances | Connect-then-exit logic; only remove confirmed socket files |
| Integration tests may be flaky (timing) | Use timeouts and retries; avoid sleep-based assertions |
| OPR publication not guaranteed | Treat PKGBUILD as local artifact; OPR acceptance is separate |

---

## Confirmation

- No implementation code was written in this document.
- No `go.mod` was initialized.
- No dependencies were installed.
- No empty source-code directories were created.
- The approved technology stack (Go, Quickshell/QML, JSON-RPC 2.0 over Unix socket, TOML, SQLite deferred) is unchanged.
- No features beyond Phase 1 scope are included.
- No architecture decisions were silently changed.

This plan is ready for review before implementation begins.


## M16 and M17 — Implemented utility increments before release

These user-requested increments landed before M15 release work. M16 adds panel
keyboard/scrolling behavior, demand-aware system sampling, and timed Keep Awake
([ADR-0011](docs/architecture/ADR-0011-keep-awake.md)). M17 adds playback-stream
volume/mute, default-output selection, System/Audio navigation, live updates,
and audio-service recovery ([ADR-0012](docs/architecture/ADR-0012-audio-mixer.md)).
Go unit/integration and race tests, QML tests, real UI lifecycle checks, and an
isolated audio-stack smoke test cover these increments. Per-app output routing,
master-output volume, microphone controls, volume boost, and persistent app
levels are not part of M17. M15 release/install verification is now complete; see the release record below.


## M18 — Playback-stream output routing (implemented)

Adds `audio.moveStream` and a device selector on each playback row, independently
of the system default output. Validate both targets against current server state,
preserve volume/mute/unrelated streams, and handle failures without retaining an
unconfirmed UI selection. The shared picker preserves open menus across ordinary
refresh and closes them on device-list changes. Verification includes fake-driver
and RPC permission tests, QML keyboard/hotplug/rollback tests, and the isolated
PipeWire test with two concurrent streams. No persistent routing rules or app-wide
grouping are introduced. See [ADR-0013](docs/architecture/ADR-0013-stream-routing.md).


## M19 — Microphone controls and recording apps (implemented)

Extends audio snapshots with inputs, default input, and recording-app/source
metadata. Adds named-source gain/mute and default-input methods under the existing
capability. The Microphone view excludes speaker monitors from mutation controls,
shows recording sources, and loads without opening a capture stream. Gain/mute
queues preserve commands for distinct inputs; input/output selectors share the
same stable-model control. Tests cover source parsing, grants/validation,
unchanged unrelated state, missing devices, and FIFO-backed virtual sources with
a recorder discarding test data. Live sound meters and recording-app routing are
not part of this milestone. See [ADR-0014](docs/architecture/ADR-0014-microphone-controls.md).


## M20 — Session clipboard history (implemented)

Adds an explicitly started, memory-only text collector using wl-clipboard. Each
backend session starts paused; read and write grants are independent and denied
by default. History supports stable-ID deduplication, local search, copy-back,
delete, clear, and pause. Collection continues behind a hidden panel. Bounds are
50 entries, 16 KiB/item, and 128 KiB total. Marked-sensitive selections, binary
content, files, and images are skipped; unmarked secrets may still be collected.
Go race/permission/bounds tests, QML controls/cache tests, and a private nested
Wayland test cover collection, exact copy, pause, hidden UI, and restart. No
persistent storage, automatic paste, or favorites are introduced. See
[ADR-0015](docs/architecture/ADR-0015-clipboard-history.md).


## M21 — Clipboard favorites (implemented)

Adds Save favorite/Unsave, saved badges, and All copies/Favorites filters. Only
explicit favorites persist in private, atomically replaced JSON; recent copies
stay session-only. Clear recent and eviction preserve saved entries. Remove
updates disk before removing a favorite; failed writes preserve confirmed state.
Twenty favorites and 32 KiB of saved text fit within the existing list/frame
bounds. Corrupt/unsafe files remain untouched and are reported while recent
history stays usable. Existing read/write grants are unchanged; startup remains
paused. Go race/permission/storage tests, 94 QML tests, and private nested Wayland
restart/copy/removal checks verify the increment. See
[ADR-0016](docs/architecture/ADR-0016-clipboard-favorites.md).


## M22 — Session File shelf (implemented)

Adds a static `file_shelf` module and Files page with explicit path/URL input,
copy-only drop handling, search, metadata/status, Open folder, Copy path,
Remove, and Clear shelf. References are memory-only and bounded to 50 entries,
4096 bytes/path, and 32 KiB total paths. Original contents stay unchanged; batch
validation is atomic and missing paths recover without losing row identity.
Every method requires `filesystem.access`; copy additionally requires
`clipboard.write`. Shared visible-only snapshot requests support independent
clipboard/file-shelf permissions. Go race/permission/bounds tests, 109 QML tests,
private nested Wayland copy/lifecycle checks, and recorded folder-opener
arguments verify the increment. Native file-manager drag delivery still needs
manual verification. Persistence and file transfer operations remain deferred.
See [ADR-0017](docs/architecture/ADR-0017-file-shelf.md).


## M23 — Clipboard utilities (implemented)

Adds fixed Transform & copy operations on recent entries and favorites. Resolve
full source text by ID, transform in memory, and copy with the existing read/write
grants and deadline. Source text and saved files remain unchanged; active
collection can capture the derived result normally. Empty/oversized outputs and
invalid operations never touch the clipboard. Expandable keyboard controls fit
the existing panel and preserve focus through refresh. Verification includes Go
race/permission/Unicode/bounds/source-preservation tests, 112 QML tests, and real
private Wayland copies with paused/active collection and rejected results.
No new dependency, config grant, persistence format, or automatic paste is added.
See [ADR-0018](docs/architecture/ADR-0018-clipboard-transforms.md).


## M24 — Running-window switcher (implemented)

Adds the static window_switcher module and Windows page under window.manage.
Queries mapped input-capable ordinary clients only while visible; bounds labels,
raw replies, and displayed windows. Focus requires a strict live address and
post-dispatch confirmation. Search supports apps, titles, and workspaces; Enter
switches to the first result. Successful focus hides the panel, errors keep it
open. Titles remain in memory and frontend caches clear on hide/disconnect.
Go race/permission/parser/limit tests, 123 QML tests, and a private nested Hyprland
test verify focus across workspaces, active confirmation, closed targets, and
empty desktops. Launching apps, previews, special workspaces, and layouts remain
deferred. See [ADR-0019](docs/architecture/ADR-0019-window-switcher.md).


## M15 — Released (2026-11-13)

The x86_64 working-tree candidate passed Go build/vet/race checks, all 123 QML
checks, real UI/reconnect checks, isolated audio and private shelf/window tests,
and real Keep Awake cleanup. The Arch package built and passed an extracted
package UI smoke test, then was installed over the existing same-version
package. The installed service runs `/usr/bin/omnibar` without the temporary
development override; live window focus was confirmed after restart. Draft
release notes and the exact candidate checksum are recorded in
[the verification record](docs/releases/v0.1.0-verification.md).

M15 is complete for x86_64: `v0.1.0` is tagged and published with an Arch
package, source archive, checksums, and verified PKGBUILD. The published tag
archive passed a clean makepkg build and package UI smoke test. Isolated
upgrade/reinstall/downgrade/removal checks preserved config/data/state; the
final package is installed and the packaged backend/UI were restarted.
aarch64 and fresh-machine/reboot testing remain documented limits; OPR
publication remains separate.


## M25 — Installed application launcher (implemented)

Adds an Apps page and `application_launcher` module with bounded XDG desktop
entry discovery, user/system precedence, visibility filters, localized labels,
search, and explicit launch via GIO. Both RPC methods require `process.manage`;
the example leaves it false. Launch accepts a freshly validated desktop ID and
hides the panel only when GIO accepts the request. Stable rows retain keyboard
focus, and catalog polling runs only while visible. No startup launch, watchers,
raw-command RPC, or history storage is introduced. Go/QML tests and private
Hyprland checks cover real button/Enter GUI launches, argument expansion,
failure retention, and stale entries. All 134 QML checks and Go race tests
passed. The local package passed UI smoke and isolated upgrade/rollback/remove
checks against v0.1.0, then was installed: 53 files with zero alterations and
71 live catalog entries without truncation. `glib2` supplies the GIO dependency.
See [ADR-0020](docs/architecture/ADR-0020-application-launcher.md). Development
now targets 0.2.0; v0.1.0 remains the first published release.


## M26 — Unified app/window search (implemented)

Adds Search in the panel header, Ctrl+K within the focused panel, and
`panel showSearch`. The existing app/window controllers supply independent
permission-aware snapshots; partial availability remains usable. Multi-token
matching searches the source catalogs and shows up to 30 results, with running
matches first and clear Switch/Launch actions. Arrow navigation scrolls the
selection into view; row identity and explicit choices survive refreshes.
Search waits for available sources before focusing the field, clears its query
and rows on hide, and uses existing confirmed-focus/accepted-launch success
signals for dismissal. No new backend module, capability, dependency, or search
history is introduced. See [ADR-0021](docs/architecture/ADR-0021-unified-search.md).


M26 verification: all 144 QML tests and Go vet/race checks passed. The real
private Search test covered shortcut opening, failed-launch retention,
arrow-selected GUI launch, exact confirmed focus, and reset on reopen. Layout
captures verified both the query and a scrolled selection. The package passed
UI smoke and full-payload upgrade/downgrade/remove checks against M25. Local
0.2.0-2 is installed with 55 files and zero alterations; both sources respond
(71 apps and 3 windows at verification), and Search is open in the packaged UI.
