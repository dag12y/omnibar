# Changelog

All notable changes will be documented here.

## [Unreleased]

### Added
- Initial project documentation.
- Initial development roadmap.
- Initial requirements.
- Initial architecture proposal.
- Go project scaffold, `cmd/omnibar` entrypoint, and Makefile (M1).
- `log/slog` logging and TOML configuration with defaults, versioning, and validation (M2).
- Unix domain socket server with `0700` runtime directory, `0600` socket, stale-socket recovery, single-instance lock, newline-delimited framing, and graceful shutdown (M3).
- JSON-RPC 2.0 dispatcher with `client.handshake` and `module.getStatus`, protocol version `omnibar.ipc/v1`, and error codes (M4).
- Static module registry with lifecycle states, panic recovery at module boundaries, fail-closed capability checks, and the `core` module (M5).
- Quickshell QML UI with a socket client, handshake, connection status, offline mode, and exponential-backoff reconnection (M6).
- Integration test harness that spawns the real backend binary (`tests/integration`), covering handshake, errors, notifications, permissions, shutdown, restart, crash recovery, and single-instance locking (M7).
- `make e2e-ui` / `scripts/e2e-ui.sh`: runs the real Quickshell UI against the real backend and checks connect, handshake, offline backoff, and reconnect (M7).
- Local development guide at `docs/development/local-development.md` (M7).
- MIT `LICENSE`.
- `omnibar --version`.
- Arch packaging (M8): `pkgbuild/PKGBUILD`, the `omnibar-ui` launcher, the `systemd/omnibar.service` user unit, and `make package` / `scripts/package-local.sh` to build a package from the working tree.
- `tests/packaging`: keeps `pkgver` equal to the backend version and checks the unit file, installed files, example config, and that no self-updater exists.
- Packaging, install, upgrade, rollback, uninstall, and release notes in `docs/development/packaging.md`.
- Event subscription and delivery (M9, ADR-0009): `internal/events` hub, `module.subscribe` / `module.unsubscribe`, `module.event` notifications, per-connection sessions with a bounded outbound queue (a slow client is disconnected, never blocking publishers), error codes -32001 (permission denied) and -32002 (module not available).
- `Dispatcher.RegisterContext` / `HandleContext`, `ipc.NewWithContext`, `ipc.SessionFromContext`, and `Session.OnClose`.
- UI: subscribes to every module after each handshake, tracks per-module subscription state (shown as a dot on each module chip), and emits `moduleEvent`; pure-JS `protocol.js` with Qt Test coverage.
- `make e2e-ui` now also checks that the UI subscribes and re-subscribes after a backend restart.

### Changed
- The integration harness finds the repository root from the working directory, so it also works under `-trimpath` (package `check()`).
- Documented reconnect backoff consistently as 1s, 2s, 4s, 8s, 16s, then capped at 30s.

### Deferred
- `module.subscribe` and `module.event` notifications until the first event-producing module (Phase 2).
- A production RPC method that consumes a capability; the fail-closed path is tested with a test-only module.
