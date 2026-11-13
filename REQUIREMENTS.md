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
- Grouped app controls, master-output
  volume, boost, and persistent app levels remain future work.


### Stream routing acceptance (implemented)

- Validate the live stream and destination before moving; reject stale targets.
- Preserve volume, mute, default output, and unrelated streams.
- Keep output pickers stable on ordinary refresh, close stale device menus on
  hotplug, and show an unavailable current destination explicitly.
- Reconcile selections with confirmed state after success or failure.
- Store no automatic routing rules; existing audio-session policy owns fallback
  and persistence. Physical Bluetooth transitions still need hardware testing.


## Microphone increment (implemented)

- Select the default non-monitor microphone, set gain from 0–100%, and mute/unmute.
- List recording-stream app identity, current source, and paused/open metadata;
  distinguish microphone sources from speaker/system-audio monitors.
- Reuse `audio.control` and reject missing/stale inputs, malformed settings,
  disabled modules, and monitor targets before mutation.
- Preserve unrelated microphone and playback settings. Selecting a default input
  issues no explicit volume/mute or recording-stream move command.
- Open the Microphone view without recording audio or changing settings; gain is
  a control value rather than a sampled sound-level meter.
- Handle absent devices, audio-service restarts, and unavailable/cached metadata.
- Test using isolated FIFO-backed sources and discarded virtual recording data,
  preserving the user's actual audio services and microphone settings.
- Per-recording-app input routing, live meters, noise suppression, global
  all-microphone mute, and hardware-specific verification remain future work.


## Clipboard increment (implemented)

- Start paused each backend session, requiring both a read grant and explicit Start.
- Collect UTF-8 text in memory, deduplicate by exact contents, and cap history at
  50 entries, 16 KiB/item, and 128 KiB total; evict oldest entries first.
- Search locally, copy exact text with a separate write grant, delete, clear, and
  pause. Clear/delete leave the system clipboard unchanged; no injected paste.
- Continue opted-in collection behind hidden pages; clear frontend caches when
  hidden/disconnected and forget backend history on shutdown.
- Skip sensitive-marked and unsupported content without reading it into history.
  Unmarked secrets cannot be reliably identified. Ordinary history has no
  persistence or content logs; images, files, and app exclusions deferred. M21 adds
  explicit favorite persistence as described below.
- Verify real behavior using synthetic data in a separate Wayland compositor.


## Clipboard favorites increment (implemented)

- Save only explicitly selected favorites across restarts, retaining exact text.
- Restore favorites while keeping collection paused; require the existing read
  grant before loading or accessing saved clipboard content.
- Limit favorites to 20 entries and 32 KiB of text within the shared 50-entry,
  128 KiB list budget. Evict only recent entries; Clear recent keeps favorites.
- Unsave removes persisted text but leaves a recent item; Remove deletes from
  both disk and list. Require the write grant only for copying to the clipboard.
- Save atomically with private permissions. Preserve confirmed state on write
  failure and preserve invalid files without silently overwriting them.
- Display saved badges, a Favorites filter, local search, and storage errors.
- Use isolated data directories and synthetic text for tests; no full persistent
  clipboard history or encrypted storage is introduced.


## File shelf increment (implemented)

- Hold explicit session references to local regular files and directories,
  accepting absolute paths or local file URLs and copy-only URL drops.
- Validate batches atomically, retain stable IDs on duplicate/reordered paths,
  and reject remote, unsupported, malformed, or missing add targets.
- Bound references to 50 entries, 4096 bytes/path, and 32 KiB total path bytes.
- Show name/path/type/size and unavailable status; recover when paths return.
- Search locally, open containing folders, copy exact paths, remove references,
  and clear the shelf without modifying or reading original contents.
- Gate all methods with `filesystem.access`, copy with `clipboard.write` too;
  keep example grants false and reject access before filesystem/desktop calls.
- Clear references on backend shutdown and frontend caches on hide/disconnect;
  refresh metadata only while visible. No recursive scans or file watchers.
- Verify with synthetic files/private Wayland, fake opener arguments, and
  permission/UI tests. Persistent shelf, file payload copying, drag-out, and
  destructive file operations remain deferred.


## Clipboard utilities increment (implemented)

- Transform & copy full backend-owned source text by ID using fixed trim,
  lowercase, uppercase, and remove-line-breaks operations.
- Preserve source/favorite text and collection settings; active collection can
  capture derived results under normal deduplication and eviction bounds.
- Trim outer Unicode whitespace, use simple locale-independent Unicode case
  mappings, and replace line breaks with spaces without merging adjacent words.
- Gate with both existing clipboard grants. Reject unknown/malformed operations,
  stale IDs, empty output, and UTF-8 expansion beyond the item byte limit.
- Keep clipboard state on pre-copy rejection, propagate driver failures, and use
  existing copy deadlines without automatic paste or arbitrary expressions.
- Provide keyboard-accessible expandable actions, read-only/busy states, and
  stable source focus. Verify synthetic real copies and untouched saved files in
  the private Wayland test.


## Running-window switcher increment (implemented)

- Query existing mapped, non-hidden input-capable Hyprland windows across ordinary
  workspaces, with class/title/workspace/monitor/active metadata.
- Search app/title/workspace locally, support keyboard Enter and Switch controls,
  retain stable row focus, and hide only after confirmed target focus.
- Require window.manage before any compositor call. Validate exact hexadecimal
  addresses against a fresh list; reject stale targets and selector/command input.
- Bound raw replies, label bytes, displayed windows, and RPC deadlines. Show
  explicit unavailable, denied, busy, empty, and truncated states.
- Require an explicit compositor instance; poll only while visible and retain
  no persistent window history or title logs. Clear frontend caches on hide.
- Verify cross-workspace focus and closed/invalid targets using synthetic windows
  in a private compositor; leave host focus/config and application launching alone.
- Previews, layouts, special-workspace/group restoration, and non-Hyprland support
  are deferred.


## Application launcher increment (implemented)

- Discover installed XDG desktop applications with user override precedence,
  visibility and TryExec filters, and localized labels.
- Search name/description/keywords, launch with a button or Enter, and close the
  panel only when the desktop launch request is accepted.
- Require process.manage for listing and launching; deny before filesystem or
  desktop work. Accept only a freshly listed desktop ID, never arbitrary commands.
- Delegate desktop launch semantics to GIO, using a single exact file argument
  and no shell. Preserve explicit failures without treating acceptance as proof
  that an app produced a window.
- Bound scans, metadata, snapshots, and deadlines; retain no launch history.
  Refresh only while visible and clear caches on hide/disconnect/shutdown.
- Verify real argument expansion and button/Enter GUI launches with synthetic
  desktop entries in a private compositor. Icons, history, desktop actions,
  URI/file launch arguments, and unified window/app search remain deferred.
