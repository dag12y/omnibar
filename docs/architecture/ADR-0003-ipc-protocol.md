# ADR-0003: IPC Protocol — JSON-RPC 2.0 over Unix Domain Socket

Status: Accepted

## Context

The Go core services and the Quickshell QML UI run in separate processes. They need a communication mechanism. Go has no official or widely-maintained QML bindings, so an out-of-process IPC boundary is required. The original architecture left the bridge mechanism undefined.

## Decision

Use **newline-delimited JSON-RPC 2.0 over a Unix domain socket**. Go owns the socket server; the QML UI connects as a client using `Quickshell.Io.Socket` from `Quickshell.Io`. `SocketServer` is not used for this direction of communication.

## Alternatives considered

### Quickshell QML `Socket` (chosen)

Quickshell exposes `Quickshell.Io/Socket` QML type, plus `SocketServer` and `IpcHandler` in the same module.

- Go listens on the socket; QML `Socket` connects to it as a client.
- Messages are newline-delimited JSON-RPC 2.0.
- No C++ glue code needed on the QML side.
- `SocketServer` is not used because Go owns the server side.

**Advantages:** Zero C++ code on the UI side; uses Quickshell types already installed; clean process boundary; debuggable with `socat`/`nc`.

**Disadvantages:** Requires a small amount of framing logic (read until newline, enforce max message size).

### Custom C++ QLocalSocket wrapper

Write a C++ QML plugin exposing `QLocalSocket` to QML.

**Advantages:** Full control over the socket API.

**Disadvantages:** Requires C++ plugin, build integration, and ongoing maintenance; more complex than using Quickshell's built-in QML types.

**Decision:** Not needed. Quickshell's `Socket` QML type is sufficient.

### Stdio JSON-RPC

Launch the Go backend as a subprocess; communicate over stdin/stdout.

**Advantages:** Simplest socket-path management; automatic cleanup when UI exits.

**Disadvantages:** QML has no built-in process control; needs `QProcess` or a C++ wrapper; harder to run multiple UI instances against one backend.

**Decision:** Rejected. Unix socket is more flexible and standard for long-running desktop services.

### cgo / C++ bindings to Qt

Expose Qt via cgo to Go for in-process communication.

**Advantages:** Direct in-process communication, no serialization.

**Disadvantages:** Qt must run on the main thread; Go's scheduler and garbage collector complicate thread affinity; fragile and not recommended for production.

**Decision:** Rejected. Too much complexity for marginal benefit.

## Protocol specification

### Message framing

- Each message is exactly one JSON object, terminated by a single `\n` (newline-delimited).
- The receiver reads bytes until `\n`, then parses the JSON object.
- Maximum message size is bounded (default 1 MiB). Messages exceeding the limit are rejected and the connection is closed.

This framing prevents unbounded memory growth from a malformed or malicious sender and makes stream parsing deterministic.

### JSON-RPC 2.0 rules

- Every request has an `id`. Its response uses the same `id`.
- A JSON-RPC response must **not** contain a `method` field. It contains either `result` or `error`.
- Notifications (one-way messages) have no `id` and expect no response.
- If the backend initiates a request toward the UI, it must use its own request `id`.

### Handshake examples

**Handshake request (UI → backend):**
```json
{"jsonrpc":"2.0","id":1,"method":"client.handshake","params":{"protocol":"omnibar.ipc/v1","client_id":"omnibar-ui-1"}}
```

**Handshake success response (backend → UI):**
```json
{"jsonrpc":"2.0","id":1,"result":{"protocol":"omnibar.ipc/v1","version":"0.1.0","modules":["system_monitor"]}}
```

**Handshake error — protocol version incompatibility (backend → UI):**
```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"Protocol version mismatch: expected omnibar.ipc/v1"}}
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
{"jsonrpc":"2.0","method":"module.event","params":{"module":"audio","event":"volume_changed","value":45}}
```

### Connection lifecycle

1. UI starts and attempts to connect to `$XDG_RUNTIME_DIR/omnibar/omnibar.sock`.
2. If the backend is not running, the UI shows a "Connecting…" state and retries with exponential backoff (1s, 2s, 4s, 8s, 16s, then capped at 30s).
3. On connect, the UI sends `client.handshake` with client ID and protocol version (`omnibar.ipc/v1`).
4. The backend replies with `server.handshake` including protocol version and available modules.
5. The UI subscribes to events of interest via `module.subscribe`.
6. The backend pushes notifications for subscribed events.

### Reconnection behavior

- Socket disconnection → UI enters offline mode: cached data displayed, backend-requiring actions disabled with a "Backend unavailable" message.
- Automatic reconnect attempts with exponential backoff.
- On reconnect, UI re-sends handshake and re-subscribes to events.
- Pending requests that timed out are reported as errors to the UI.

### Error handling

- **Standard JSON-RPC error codes** (-32700 through -32603) are reserved for standard protocol errors:
  - -32700: Parse error
  - -32600: Invalid request
  - -32601: Method not found
  - -32602: Invalid params
  - -32603: Internal error
- **Server-defined custom errors** use the range -32000 through -32999.
- Backend errors include a `module` field for routing when applicable.
- UI shows non-blocking error toasts for transient errors; critical errors are shown inline.

### Protocol versioning

- Handshake exchanges protocol version `omnibar.ipc/v1`.
- Mismatched major versions are rejected with a standard JSON-RPC error (code -32600).
- Minor version differences are tolerated with deprecation warnings.

### Socket security

- **Socket directory:** `$XDG_RUNTIME_DIR/omnibar/`, created with mode `0700`, owned by the running user.
- **Socket file:** `$XDG_RUNTIME_DIR/omnibar/omnibar.sock`, created with mode `0600`, owned by the running user.
- No separate authentication: single-user desktop, socket in the user's runtime directory. The user running the UI is the same user running the backend.
- If `$XDG_RUNTIME_DIR` is unset or unavailable, the backend logs a fatal error and exits.

### Stale socket handling

- Before binding, the backend checks whether a socket file already exists at the socket path.
- If a socket file exists, the backend attempts to connect to it. If the connection succeeds, a live backend is already listening; the new instance logs an error and exits to avoid conflicts.
- If the connection fails (e.g., `ECONNREFUSED`), the file is a stale socket left by a crashed process. The backend removes only that specific socket file path after confirming it is a socket (not an arbitrary filesystem entry) and then binds.
- The backend never blindly deletes an arbitrary filesystem entry at the socket path.

### Graceful shutdown

- On `SIGTERM` or `SIGINT`, the backend stops accepting new connections, closes existing client connections, stops all modules, removes the socket file, and exits with code 0.
- On `SIGKILL`, the socket file may be left behind; the stale-socket handling above manages this on the next startup.

### Graceful degradation

The UI must continue functioning when the backend is unavailable: cached data is shown, actions requiring the backend are disabled, and a reconnect indicator is displayed. The UI never crashes due to backend absence.

## Consequences

**Positive:**
- Clean UI/backend process boundary → failure isolation (a UI crash does not kill the backend, and vice versa)
- Language-agnostic and debuggable with standard tools
- No CGo or Qt/Go threading complexity
- Bounded message size protects against memory exhaustion

**Negative:**
- Serialization overhead (negligible for UI-scale messages)
- Connection lifecycle and reconnection logic must be implemented correctly
- Two processes to manage instead of one

## References

- Quickshell `Quickshell.Io` QML types (installed locally at `/usr/lib/qt6/qml/Quickshell/Io/`)
- JSON-RPC 2.0 specification: https://www.jsonrpc.org/specification
- Qt6 `QLocalSocket` documentation (fallback reference): https://doc.qt.io/qt-6/qlocalsocket.html
