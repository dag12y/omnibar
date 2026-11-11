# Local development and end-to-end verification

This guide covers running the Omnibar backend and Quickshell UI as separate
processes, exercising the connection lifecycle manually, and running the
automated integration checks.

## Prerequisites

- Go 1.27 or newer.
- Quickshell 0.3.2 (the pinned UI version).
- Qt 6, including `/usr/lib/qt6/bin/qmltestrunner`.
- A Wayland session for the graphical UI (Hyprland on Omarchy).

Everything runs as the invoking user. Omnibar never uses `sudo`.

## Build

```sh
make build
# equivalent to
go build -o build/omnibar ./cmd/omnibar
```

## Run the backend

```sh
./build/omnibar
# or
make run
```

The backend:

- reads `$XDG_CONFIG_HOME/omnibar/config.toml` (or
  `~/.config/omnibar/config.toml`), and uses defaults if the file is missing;
- creates `$XDG_RUNTIME_DIR/omnibar/` with mode `0700`;
- listens on `$XDG_RUNTIME_DIR/omnibar/omnibar.sock` with mode `0600`;
- logs structured `slog` records to stderr.

`XDG_RUNTIME_DIR` is mandatory. There is no fallback to `/tmp`.

A minimal configuration is enough to get started:

```sh
mkdir -p "${XDG_CONFIG_HOME:-$HOME/.config}/omnibar"
cp config/config.toml.example "${XDG_CONFIG_HOME:-$HOME/.config}/omnibar/config.toml"
```

Edit the permissions table to grant or deny capabilities. Missing entries are
denied (fail-closed). Permission changes require a backend restart in Phase 1.

## Run the UI

In a second terminal:

```sh
quickshell -p ui/qml/shell.qml
```

The UI connects to the backend socket, sends `client.handshake`, and should
show **Connected** with the `core` module. It does not build or bundle QML;
Quickshell loads the files directly.

If the UI starts before the backend, it shows **Backend unavailable** (or
**Connecting…**) and retries with exponential backoff: 1s, 2s, 4s, 8s, 16s,
then every 30s.

## Manual end-to-end checklist

Use this sequence to verify the M7 acceptance criteria:

1. Start the backend with `./build/omnibar`.
2. Start the UI with `quickshell -p ui/qml/shell.qml`.
3. Confirm the UI shows **Connected** and lists `core`.
4. Stop the backend with `Ctrl+C` (`SIGINT`) or `kill -TERM <pid>`.
   - The process exits with status 0.
   - The socket file is removed.
   - The UI switches to offline mode and keeps cached module data.
5. Start the backend again.
6. Confirm the UI reconnects on its own, re-sends the handshake, and returns to
   **Connected**. The reconnect may take up to 30 seconds because the backoff
   is intentionally capped.
7. Run the automated checks below.

You can also inspect the protocol without the UI:

```sh
SOCKET="${XDG_RUNTIME_DIR}/omnibar/omnibar.sock"
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"client.handshake","params":{"protocol":"omnibar.ipc/v1","client_id":"manual"}}' \
  | socat - UNIX-CONNECT:"$SOCKET"
```

Expected response:

```json
{"jsonrpc":"2.0","id":1,"result":{"protocol":"omnibar.ipc/v1","version":"0.1.0","modules":["core"]}}
```

Check socket security:

```sh
ls -ld "${XDG_RUNTIME_DIR}/omnibar"
ls -l "${XDG_RUNTIME_DIR}/omnibar/omnibar.sock"
# directory: drwx------ (0700)
# socket:    srw------- (0600)
```

## Automated tests

Run everything:

```sh
make test
```

Run only the end-to-end integration tests with verbose output:

```sh
go test ./tests/integration -v
```

The integration harness builds the real `cmd/omnibar` binary in `TestMain` and
spawns it with isolated `XDG_RUNTIME_DIR` and `XDG_CONFIG_HOME` values. Tests
cover:

- handshake request/response with matching IDs, protocol version, and module
  list;
- request/response round trips and JSON-RPC error codes;
- notifications (no `id`) producing no response;
- socket and runtime-directory permissions;
- graceful shutdown (`SIGTERM`, exit 0, socket removed);
- backend restart and re-handshake over the same socket path;
- crash restart with stale-socket recovery;
- single-instance locking;
- module disablement;
- fail-closed capability checks.

Run the QML logic tests separately:

```sh
make ui-test
```

This runs the Qt Test suite under `ui/qml/tests/` with Qt 6 in offscreen mode.

### How fail-closed capability testing works

The shipped `core` module declares no capabilities, so no production RPC method
currently consumes one. The integration test in
`tests/integration/permissions_test.go` therefore registers a test-only module
that declares `system.info` and a test-only `test.useCapability` method. The
method calls the production `Registry.HasCapability` path over a real Unix
socket, with real TOML configuration loading and the production registry,
dispatcher, and framing layers.

The test verifies that access is denied when:

- the `[permissions]` table is missing;
- the permission is explicitly `false`;
- the permission is granted but the module does not declare it;
- the module is disabled;
- the module's declared capability has no matching grant.

Access is granted only when the module declares the capability, the user
grants the matching permission, and the module is enabled and healthy. A
production capability-consuming method will replace the test-only method when
the first such module lands.

### Subscription/notification scope

`ARCHITECTURE.md` and ADR-0003 describe a future `module.subscribe` method and
backend-pushed `module.event` notifications. No milestone in the Phase 1
foundation defines event payloads or an event-producing module, so the backend
does not implement `module.subscribe` yet.

The integration tests verify the notification behavior that does exist:
JSON-RPC messages without an `id` are dispatched and produce no response, and
reconnecting clients perform a fresh handshake. Subscription and event
delivery should be added together with the first event-producing module.

## Troubleshooting

### `XDG_RUNTIME_DIR is not set`

Run from a graphical login session, or export a private runtime directory:

```sh
export XDG_RUNTIME_DIR="/run/user/$(id -u)"
```

Do not point `XDG_RUNTIME_DIR` at a shared or world-writable directory.

### `another instance is already running`

A backend already holds the single-instance lock. Stop the old process:

```sh
pkill -TERM -x omnibar
```

The lock file (`omnibar.lock`) is intentionally not deleted on shutdown to
avoid inode races. Its presence alone does not mean an instance is running.

### The UI stays on "Backend unavailable"

- Check that the socket exists and is a socket:
  `ls -l "${XDG_RUNTIME_DIR}/omnibar/omnibar.sock"`.
- Check the backend log for startup or listen errors.
- Check that the runtime directory is mode `0700`; the backend refuses unsafe
  directory permissions.
- Remember that reconnect backoff can take up to 30 seconds.

### Wrong Qt test runner

The `qmltestrunner` on `PATH` may be Qt 5. Use `make ui-test`, which defaults to
`/usr/lib/qt6/bin/qmltestrunner`, or set `QMLTESTRUNNER` explicitly.

### Stale socket

A crashed backend can leave a socket file behind. The next startup detects a
refused connection, removes only confirmed stale Unix sockets, and binds
normally. Regular files, directories, and symlinks are never removed.

## Related documents

- [Development guidelines](../../DEVELOPMENT.md)
- [Architecture](../../ARCHITECTURE.md)
- [IPC protocol ADR](../architecture/ADR-0003-ipc-protocol.md)
- [Permissions ADR](../architecture/ADR-0007-permissions-security.md)
