# Product Requirements

## Target platform
- Primary: Omarchy Linux
- Window manager: Hyprland
- Future: Other Arch Linux and Wayland environments

## Phase 1 MVP definition

The Phase 1 minimum viable product is the core foundation that proves the architecture works end-to-end:

1. Go backend starts and listens on a Unix domain socket.
2. Quickshell UI launches.
3. UI connects to Go through the Unix socket using `Quickshell.Io.Socket`.
4. JSON-RPC handshake and request/response work between UI and backend.
5. Basic module registry works (static registration of built-in Go modules).
6. TOML configuration loads and validates (application-defined validation).
7. Logging (Go `log/slog`) works.
8. Graceful shutdown works (socket cleanup, module stop).
9. A minimal working UI demonstrates the connection.

### Phase 1 acceptance criteria

- The Go backend starts without errors and creates the socket at `$XDG_RUNTIME_DIR/omnibar/omnibar.sock`.
- The Quickshell UI launches and connects to the backend socket.
- A JSON-RPC request from the UI receives a matching-ID response from the backend.
- The backend shuts down gracefully on `SIGTERM`, removing the socket file.
- Configuration with an invalid permission grant fails closed (access denied).
- Unit tests pass for the module registry, IPC framing, and configuration validation.

## Phase 1 non-goals (explicitly out of scope)

- SQLite database (no driver, no tables, no migrations in Phase 1)
- System monitoring (CPU, RAM, etc. — deferred to Phase 2)
- Window management (deferred to Phase 3)
- Clipboard (deferred to Phase 4)
- Audio control (deferred to Phase 5)
- Screen capture (deferred to Phase 7)
- Process/package management (deferred to Phase 8)
- Dynamic Island-style activities (deferred to Phase 9)
- Developer tools (deferred to Phase 10)
- Self-updater (not planned; updates via Arch package management)
- OS-level security sandboxing (future work)
- External plugin/extension support (future work)

## Functional requirements
- Each feature must be independently configurable.
- Modules must be enabled and disabled independently.
- Settings must persist between restarts.
- Background services must avoid unnecessary resource usage.
- Modules must communicate through documented interfaces.
- The application must support updates.
- User data must survive application updates.

## Non-functional requirements
- Reliable operation.
- Low CPU and memory overhead.
- Secure system integration.
- Accessible and consistent UI.
- Maintainable code.
- Automated testing.
- Clear documentation.

## Privacy
- Local-first by default.
- No unnecessary telemetry.
- No collection of personal data without consent.
