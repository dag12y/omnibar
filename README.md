# Omnibar

A modular desktop utility suite for Linux, inspired by
Vorssaint and designed initially for Omarchy and Hyprland.

## Vision

Bring system monitoring, window management, productivity,
media utilities, developer tools, and desktop controls
together in one unified application.

## Principles

- Modular and extensible architecture.
- Independent and configurable features.
- Native Linux integration.
- Privacy-first and local-first.
- Reliable updates and rollback support.
- Well-tested and maintainable code.

## Project status

Phase 1 (core foundation) is complete: a Go backend and a Quickshell UI
connect over a Unix socket using JSON-RPC 2.0, with a module registry,
TOML configuration, and an Arch package with a systemd user service.
System monitoring is implemented: CPU, RAM, swap, disk capacity and I/O,
network throughput, and battery status/power. The floating Quickshell dashboard
uses the active Omarchy theme, live CPU/memory graphs, and expandable details.
Timed Keep Awake, an audio mixer, microphone controls, and session clipboard
history with saved favorites are also implemented. System, Audio, and Clipboard
share the same panel alongside a session File shelf and a Hyprland window
switcher, app launcher, unified search, optional previews, exact window controls,
and saved workspace layouts. [v0.2.0 is available](https://github.com/dag12y/omnibar/releases/tag/v0.2.0),
with [release verification](docs/releases/v0.2.0-verification.md) for x86_64.

Run the backend with `make run` and launch the panel with
`quickshell -n -p ui/qml/shell.qml`. Click the CPU or memory card for details.
The panel opens at startup; its close button hides it and pauses page updates.
Active Keep Awake sessions and explicitly started clipboard collection continue.
Use `./scripts/toggle-panel.sh` during development, or the installed
`omnibar-toggle` command from a keybinding or bar action. Direct IPC also works:

```sh
quickshell -p /absolute/path/to/omnibar/ui/qml/shell.qml ipc call -- panel toggle
```

The same IPC target supports `show` and `hide` after the `--` separator.
Escape hides the panel; Tab and Enter/Space navigate and activate controls.
Hiding pauses the panel’s system-monitor subscription. Omarchy colors are read from
`$XDG_STATE_HOME/omarchy/current/theme/colors.toml` (default
`~/.local/state/omarchy/current/theme/colors.toml`), with a built-in fallback.
Graphs retain up to 40 readings in memory and reset on reconnect or reopening.

Keep Awake offers 15-, 30-, and 60-minute sessions with a countdown and Stop
control. It requires `"power.inhibit" = true` under `[permissions]` in your
Omnibar config (included in the example). Restart the backend after changing
grants. Sessions survive hiding the panel and end on Stop, expiry, or backend
shutdown. The Wayland display inhibitor requires the UI to remain running; the
backend holds the logind idle/sleep inhibitor. Low-battery auto-stop is not yet
implemented.

## Audio mixer

Select **Audio** to control playback streams by app name, icon, and media title.
Adjust volume from 0–100%, mute/unmute, or choose the default playback output.
Apps appear when they have an audio stream; an app with multiple streams gets
multiple rows. Paused streams may remain visible. Volume drags apply on release;
arrow keys adjust a focused slider. Each row's **OUTPUT** selector moves just
that playback stream to another connected device, preserving its volume/mute
and leaving the default output and other streams alone. The separate **DEFAULT
OUTPUT** selector changes the system default; movement of existing streams then
follows the session manager's policy.

A disappeared device displays "Output unavailable" until the audio server picks
a fallback or you select another output. Omnibar stores no routing rules; new
streams and device reconnections follow the audio server/session manager policy.

The mixer uses `pactl` with PipeWire's `pipewire-pulse` service. On Arch, `libpulse`
provides `pactl`; both are package dependencies. Grant `"audio.control" = true`
under `[permissions]` and restart the backend (the example config includes it).
No audio is changed merely by opening the page. Missing grants show an
explanation; unavailable audio services reconnect automatically. Audio watchers
stop when the page is hidden or you switch back to System.

Run `make e2e-audio` for a real backend/UI test using isolated PipeWire services,
virtual outputs, and silent test streams. Your normal playback is left alone.
See [the audio design and protocol](docs/architecture/ADR-0012-audio-mixer.md)
and [stream routing](docs/architecture/ADR-0013-stream-routing.md).

## Microphone controls

Open **Audio → Microphone** to select the default input, adjust its gain, or
mute/unmute it. Gain is a volume setting, not a live sound meter. Speaker-monitor
sources are excluded from the microphone selector. The recording-app list shows
open streams and their sources, including system-audio captures and paused apps.
It reports audio-server metadata; Omnibar does not record microphone samples.

Microphone controls use the existing `audio.control` grant. Default-input changes
follow the audio server/session manager's policy for existing recordings. Gain
and mute target the named microphone; other devices and playback remain unchanged.
No input preferences are saved by Omnibar. Missing devices and stale metadata
are shown explicitly. Per-recording-app input routing and live meters are future
work. See [microphone design and protocol](docs/architecture/ADR-0014-microphone-controls.md).

The panel can open this view directly from a shortcut:

```sh
quickshell -p /absolute/path/to/omnibar/ui/qml/shell.qml ipc call -- panel showMicrophone
```

## Clipboard history

Open **Clipboard**, then **Start history** to collect copied text for this backend
session. Install `wl-clipboard` (included in the package dependencies), grant
`"clipboard.read" = true` under `[permissions]` in your Omnibar config, and restart
the backend first. Grant `"clipboard.write" = true` to enable **Copy**; read-only
history, search, Delete, and Clear work without that grant. Both grants remain
false in the example config.

Collection starts paused after every backend restart. Starting it can include the
current clipboard selection. It continues when the panel is hidden; **Pause**
stops collection while keeping existing entries. **Clear recent** keeps saved
favorites; **Delete** removes a recent entry.
These actions leave the system clipboard alone. Copy puts the full text
back on the clipboard; paste it normally in the destination app.

Recent history stays in memory. The combined list holds up to 50 entries,
16 KiB per item, and 128 KiB of text in total, evicting oldest unsaved entries.
Repeated text moves to the top within its group. Images,
files, binary data, and sensitive-marked selections are skipped. Apps can copy
secrets without marking them sensitive, so pause collection when needed. There
is no automatic paste. Backend shutdown forgets recent history; the current
system clipboard selection can remain available.

```sh
quickshell -p /absolute/path/to/omnibar/ui/qml/shell.qml ipc call -- panel showClipboard
make e2e-clipboard  # synthetic text in a private nested Wayland compositor
```

See [clipboard design and protocol](docs/architecture/ADR-0015-clipboard-history.md).

## Clipboard favorites

Select **Save favorite** on a copied item to save that text for future sessions.
Favorites appear first, have a saved badge, and are available through the
**Favorites** filter even while collection is paused. Search works in either
filter. The existing `clipboard.read` grant permits saving/removing favorites;
`clipboard.write` is still needed only for Copy.

**Unsave** removes the disk copy but keeps the item in recent history until it is
cleared, evicted, or the backend exits. **Remove** deletes a favorite from both
the saved file and the current list. Clear recent and automatic eviction preserve
favorites. Collection still starts paused after every backend restart.

Only explicitly saved text is written to
`$XDG_DATA_HOME/omnibar/clipboard-favorites.json` (default
`~/.local/share/omnibar/clipboard-favorites.json`). This is an unencrypted JSON
file with mode 0600 in a private directory. Up to 20 favorites and 32 KiB of
favorite text are supported within the overall history limits. Save failures are
shown without changing the item; invalid saved data is preserved and reported.
See [favorites storage and protocol](docs/architecture/ADR-0016-clipboard-favorites.md).

## Clipboard utilities

On a Clipboard item, select **Transform & copy…**, then choose:

- **Trim edges**: remove whitespace at the start and end, keeping interior spacing.
- **lowercase** or **UPPERCASE**: convert letter case using Unicode mappings.
- **Remove line breaks**: replace line breaks with spaces (CRLF is one break).

The full source text is used, including text beyond the preview. The source item
and saved favorite text stay unchanged. Paste the transformed text normally in
your destination app. If collection is running, the result can appear as a new
recent copy under normal history limits; paused collection remains paused.
Empty results and results exceeding 16 KiB leave the clipboard unchanged and
show an error. The existing read/write grants are required; no config changes
or new dependencies are needed. See
[transform design and protocol](docs/architecture/ADR-0018-clipboard-transforms.md).

## File shelf

Open **Files** to keep local files and folders within reach for this backend
session. Under the existing `[permissions]` section in
`~/.config/omnibar/config.toml`, set `"filesystem.access" = true`, then restart
`omnibar.service`. Copy path additionally uses your existing `clipboard.write`
grant. The example keeps both grants false.

Enter an absolute path (such as `/home/you/Documents/notes.pdf`) or drop local
files/folders onto the drop zone. Local `file:///` URLs also work. Search by name
or path, use **Open folder** to open a directory or a file's containing folder,
and **Copy path** for its exact absolute path. Copy path copies text; it does not
copy a file payload for pasting into a file manager. **Remove** and **Clear shelf**
only remove references. Originals remain in place and backend restart clears
the shelf.

The shelf holds at most 50 items and 32 KiB of paths (4096 bytes per path).
Re-adding a path moves it to the top without duplication. Missing files are
marked unavailable, recover when the path returns, and remain removable. No
file contents, persistent shelf, directory scans, or file transfers are included.
Open folder requires `xdg-utils` and a configured desktop file manager.

```sh
quickshell -p /absolute/path/to/omnibar/ui/qml/shell.qml ipc call -- panel showFiles
make e2e-shelf  # private nested Wayland session and synthetic files
```

See [File shelf design and protocol](docs/architecture/ADR-0017-file-shelf.md).

## Running-window switcher

Open **Windows** to find an existing window by app, title, or workspace. Set
`"window.manage" = true` under your existing `[permissions]` section in
`~/.config/omnibar/config.toml`, then restart the backend. The example grant is
false. Hyprland and `hyprctl` are required, with the compositor instance available
in the backend's desktop-session environment.

Choose **Switch →**, or press Enter in search to switch to its first result.
Omnibar validates the live target and closes the panel only after Hyprland
confirms focus. Switching can take you to another workspace. Closed windows and
unavailable sessions show an error; Refresh retries. No application is launched.
Only mapped, non-hidden windows outside special workspaces are included, up to
100, with a notice if truncated. Titles refresh only while visible. Saving a layout explicitly persists its
app names and window titles; ordinary listing and previews remain in memory.
Special-workspace restoration and MRU ordering remain future work.

```sh
quickshell -p /absolute/path/to/omnibar/ui/qml/shell.qml ipc call -- panel showWindows
make e2e-windows  # synthetic windows in a private nested Hyprland instance
```

See [window-switcher design and protocol](docs/architecture/ADR-0019-window-switcher.md).

## Documentation

- [Requirements](REQUIREMENTS.md)
- [Architecture](ARCHITECTURE.md)
- [Roadmap](ROADMAP.md)
- [Development guidelines](DEVELOPMENT.md)
- [Local development](docs/development/local-development.md)
- [Packaging, install, upgrade, and rollback](docs/development/packaging.md)

## License

[MIT](LICENSE)
- [Changelog](CHANGELOG.md)


## Application launcher (0.2.0)

Open **Apps** to search installed desktop applications by name, description, or
keywords. Set `"process.manage" = true` under your existing `[permissions]` in
`~/.config/omnibar/config.toml` and restart the backend. Choose **Launch →** or
press Enter in search. Opening Apps focuses search; Tab reaches the Launch
button. The panel closes when GIO accepts the request. Launch failures keep it
open with an error. Acceptance does not guarantee the application opens a window.

GIO from `glib2` handles desktop-file commands, terminal apps, and D-Bus activation
using the backend's desktop session. User desktop entries override system ones;
hidden/unavailable entries are excluded. The visible catalog is bounded to 200
apps and refreshes every ten seconds while open; Refresh reloads immediately.
No launch history is saved. Terminal/D-Bus behavior depends on desktop setup.

```sh
quickshell -p /absolute/path/to/omnibar/ui/qml/shell.qml ipc call -- panel showApps
make e2e-apps  # real GUI launches using synthetic entries in a private compositor
```

See [application launcher design](docs/architecture/ADR-0020-application-launcher.md).


## Unified search (0.2.0)

Choose **Search** in the panel header, or press **Ctrl+K** while the panel has
keyboard focus. Type app names, window titles, descriptions, keywords, or
workspaces. Every word must match; matching running windows come first.
Use **↑/↓** to choose a result and **Enter** to activate it, or choose its
**Switch →**/**Launch →** button. Ctrl+K returns to the query and selects its
text. Selection stays stable through refreshes and scrolls into view.

Apps and windows keep their existing `process.manage` and `window.manage`
grants independently. Search remains usable when one source is unavailable.
A successful Switch requires confirmed focus; Launch closes the panel when GIO
accepts the request. Failures keep it open. Up to 30 results are shown from the
bounded source catalogs, with a notice when limits apply. Hiding the page clears
its query and results. No search history or global desktop shortcut is installed.

```sh
quickshell -p /absolute/path/to/omnibar/ui/qml/shell.qml ipc call -- panel showSearch
make e2e-search  # private app launch, keyboard selection, and window-focus checks
```

See [unified search design](docs/architecture/ADR-0021-unified-search.md).


## Window previews (0.2.0)

Install `grim` 1.5.0 or newer, enable `"screen.capture" = true` alongside
`"window.manage" = true` in your existing config, and restart the backend.
Choose **Preview** on a running-window row in **Windows** or **Search**. A
snapshot appears inside that row; **×** clears it and Preview refreshes it.
Only one preview is kept at a time. Capture leaves focus and workspace intact.

Previews use exact-window capture on Hyprland; unsupported or protected targets
show an error. Images are small JPEG thumbnails held in memory. Opening pages
or moving selection does not capture frames. Hiding the panel, changing pages,
disconnecting, or removing the target clears the image. A pending one-shot frame
can finish after hiding, but its reply is discarded. No image file or history is
saved. Switching/launching still works independently of preview failures.

```sh
make e2e-previews  # synthetic off-workspace capture, decoded pixels, and UI checks
```

See [window preview design](docs/architecture/ADR-0022-window-previews.md).


## Window controls (0.2.0)

Open **Windows → Controls** on a window. **Float/Tile** changes its mode,
**Maximize/Restore size** changes maximization, and **Move** sends it to the
entered global Hyprland workspace ID (1–9999). Changes keep the panel open.
Controls require window.manage; pinned/grouped windows are excluded. Exit
fullscreen first, and restore maximization before changing floating mode.
Targeted maximization requires a Lua-configured Hyprland session.

`make e2e-controls` verifies background targets and real controls privately.


## Saved window layouts (0.2.0)

In **Windows → Saved layouts**, enter a name and workspace ID, then choose
**Save**. Save the same name again to replace its arrangement. **Restore** moves
matching open windows back and restores floating/tiling, maximize state, and
floating positions/sizes. **Delete** removes the saved arrangement.

Open the apps first. Omnibar matches app class and title, with a class-only
fallback when exactly one window of that app exists. Missing or ambiguous
windows, changed monitors, and pinned/grouped/fullscreen targets are rejected.
Tiled windows use Hyprland's current tiling order. Restore keeps the panel open;
a compositor failure can leave a partially restored desktop, with an error.

This uses the existing `window.manage` grant. Layouts are saved only when you
choose Save, in `$XDG_DATA_HOME/omnibar/window-layouts.json` (default
`~/.local/share/omnibar/window-layouts.json`), with private permissions. The file
includes window titles. Limits are 8 layouts, 16 windows each, and 64 KiB total.
Layouts survive restarts and are never applied automatically. Corrupt data is
preserved and its error shown. Floating geometry is restored only with the same
monitor arrangement. Exact background maximize/restore requires Lua Hyprland.

See [layout design](docs/architecture/ADR-0024-window-layouts.md).


## Scratchpad (0.3.0 development)

Open **Notes**, choose **New note**, and type a title and plain text. Omnibar
autosaves after a short typing pause; wait for **Saved** before closing the UI
process. Notes survive panel close and backend/UI restarts once saved. **Pin**
keeps a note at the top, **Copy** copies its saved text, and **Delete** asks for
confirmation. Search matches titles and text; Ctrl+F focuses note search.

Enable `"notes.manage" = true` in your existing `[permissions]` config and restart
the backend. Copy also requires `clipboard.write`. The example leaves Notes
permission off. Notes use private unencrypted JSON at
`$XDG_DATA_HOME/omnibar/notes.json` (default `~/.local/share/omnibar/notes.json`).
Limits are 100 notes, 32 KiB text per note, 128-byte titles, and 2 MiB encoded data.
No arbitrary filesystem permission or network access is needed.

Save errors and conflicts keep your draft in this UI. **Retry** checks the saved
revision; **Save as new** preserves a conflicting draft as another note; **Reload
saved** asks before discarding edits. Unsaved changes cannot survive a UI process
crash. Corrupt data is preserved rather than replaced. If the Notes page is
hidden, a pending save can finish; ordinary note refreshes stop while hidden.

Use `panel showNotes` through Quickshell IPC to open this page directly.
See [Scratchpad design](docs/architecture/ADR-0025-scratchpad.md).


## Timers (0.3.0 development)

Open **Timers**, enter an optional name, and click **5m**, **15m**, **25m**, or
**60m** to start. For a custom countdown, set minutes and seconds and choose
**Start**. Pause/resume a running timer, **Cancel** an unfinished one, or
**Dismiss** a completion. The Timers button shows unfinished and completed counts.

Enable `"timers.manage" = true` in your existing `[permissions]` config and
restart the backend. The grant covers Omnibar's private timer storage and
completion alerts. It is off in the example. No arbitrary filesystem or power
inhibition permission is needed. State lives at
`$XDG_DATA_HOME/omnibar/timers.json` (default `~/.local/share/omnibar/timers.json`).

Timers continue with the panel hidden and survive UI/backend restarts. Sleep
counts toward a running timer; it does not wake your computer, and alerts arrive
after wake. Clock changes during the same boot do not change the countdown.
After reboot, recovery uses the saved UTC deadline and the system clock. Paused
timers keep their remaining time. Limits are 1 second–24 hours, 10 unfinished
timers, and the latest 20 completions.

Completion appears in Omnibar and requests one desktop notification. A missing
notification service does not lose completion. Alerts are not retried after a
restart; a crash during delivery may miss an alert, while the completion stays
saved. Corrupt or unsafe timer data is preserved and its error shown.

`panel showTimers` opens the page through Quickshell IPC. See
[timer design](docs/architecture/ADR-0026-timers.md).
