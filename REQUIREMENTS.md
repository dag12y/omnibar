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

## Phase 2 System Monitoring (M9-M15)

Phase 2 adds live system monitoring on top of the Phase 1 foundation and prepares the first release (v0.1.0):

1. The backend can push events to subscribed UI clients (ADR-0009).
2. A built-in `system_monitor` module reports system metrics (ADR-0010):
   - CPU, memory, and swap (M10, M11)
   - Disk space and I/O rates (M12)
   - Network throughput and interface transfer rates (M13)
   - Battery level, state, and power consumption (M14)
3. The module is gated by the `system.info` permission and fails closed without it.
4. The UI shows live values, marks them stale when offline, and explains denied or unavailable states.
5. The package installs, runs as a systemd user service, and the UI connects (M15 release verification).

### Phase 2 acceptance criteria

- A client that subscribes to a module receives its events; a client that does not, receives none.
- A slow or stuck client never delays the backend or other clients.
- Sampling happens only while at least one client is subscribed.
- Without a `system.info` grant, snapshots and subscriptions are denied.
- Idle backend (no subscribers) stays under 1% CPU and 20 MB RAM.
- The UI keeps working with the backend absent, restarting, or denying access.

### Phase 2 non-goals

- Persistent metric history and alerts (need storage; decided with ADR-0006). Live in-memory CPU/memory graphs are implemented.
- GPU monitoring (deferred to later Phase 2 work due to vendor-specific tooling).
- Per-process data and process management (`process.manage`, Phase 8).
- Event replay or persistence.
- OPR publication (separate, needs maintainer acceptance).

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


## Playback audio increment (implemented)

- The Audio page lists current playback streams with app/media identity and icons
  when available; distinct streams remain independently controllable.
- Users can set volume from 0–100%, mute/unmute, and select a default playback
  output. Each playback row also supports moving that stream to a chosen output
  without changing other streams or the default. Pointer volume changes commit
  on release; keyboard steps and output selection work.
- `audio.control` is required for reading, subscribing, and changing audio.
  Missing grants fail closed; disabled modules cannot contact the audio service.
- Stream/device creation and removal update the UI. An audio service restart
  recovers without restarting Omnibar. Unavailable/empty/offline states are clear.
- Background watching stops when no clients subscribe. Switching pages pauses
  the previous page's demand; active Keep Awake sessions continue independently.
- Automated tests use isolated drivers/services and do not change normal playback.
- Grouped app controls, microphone controls, master-output
  volume, boost, and persistent app levels remain future work.


### Stream routing acceptance (implemented)

- Validate the live stream and destination before moving; reject stale targets.
- Preserve volume, mute, default output, and unrelated streams.
- Keep output pickers stable on ordinary refresh, close stale device menus on
  hotplug, and show an unavailable current destination explicitly.
- Reconcile selections with confirmed state after success or failure.
- Store no automatic routing rules; existing audio-session policy owns fallback
  and persistence. Physical Bluetooth transitions still need hardware testing.
