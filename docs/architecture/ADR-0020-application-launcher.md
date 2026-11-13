# ADR-0020: Installed application launcher

Status: Accepted

## Decision

M25 adds an Apps page and static `application_launcher` module. The user can
search installed desktop entries by name, description, generic name, or keywords
and launch one with Launch or Enter. `panel showApps` opens it directly.

Read entries under the XDG data home's `applications/` directory, followed by
XDG data directories in priority order. Ignore relative XDG paths. Use desktop
file IDs, including subdirectory-to-hyphen conversion. A higher-priority ID
shadows lower-priority entries even when hidden or invalid. Apply Type,
Hidden/NoDisplay, OnlyShowIn/NotShowIn, and TryExec checks. Read the Desktop Entry
group, not action groups; use LC_ALL/LC_MESSAGES/LANG locale fallbacks for labels.
This follows the [Desktop Entry Specification](https://specifications.freedesktop.org/desktop-entry/latest-single/).

Bound each desktop file to 64 KiB and a catalog scan to 4096 visited entries.
Do not follow directory symlinks; regular desktop-file symlinks are supported.
Nonblocking file opens exclude FIFOs/devices without hanging. Display at most
200 entries sorted by name and ID, with an explicit truncation notice. IDs are
bounded to 256 bytes, labels to 128 bytes each, keeping worst-case escaped JSON
below the 1 MiB IPC frame limit. Labels are valid UTF-8 without control
characters and render as plain text. Search applies to the displayed catalog.

## Launch and permissions

Both `apps.getSnapshot` and `apps.launch {"id":"example.desktop"}` require an
enabled module and the existing `process.manage` grant. The example grant stays
false. Denied requests do not scan or launch anything. Startup creates no
watcher or launch process. RPCs use a shared three-second deadline.

Refresh the eligible bounded catalog before launch and require a matching ID.
The desktop adapter resolves the ID again immediately before dispatch, rejecting
removed or newly hidden entries. Run `gio launch` with the exact resolved
absolute desktop-file path as one argument. Accept no raw commands, arbitrary
paths, file/URI arguments, or desktop actions from IPC. GIO owns Exec field-code
expansion, terminal handling, working directories, and D-Bus activation; see
[GIO application launching](https://docs.gtk.org/gio/iface.AppInfo.html).
`glib2` provides the GIO tool and is a runtime dependency.

The launched app inherits the backend's desktop environment. Leave command
standard streams disconnected (`os/exec` uses `/dev/null`) so a long-lived app
cannot keep RPC output pipes open. A zero GIO exit status means the launch
request was accepted, not that a window appeared or the app stayed healthy.
Missing tools, unsupported terminal setup, activation failure, or timeout produce
an error and keep the panel open. Desktop entries can run their installed Exec
commands; granting access is permission to launch those applications. Live file
replacement can still occur between validation and GIO reading the entry.

## UI and lifecycle

Use the existing snapshot controller, polling every ten seconds only while the
page is visible. Refresh and page reopening discover changed installations.
Stable desktop-ID rows preserve focus; entering the page focuses search. Show
busy, unavailable, denied, disabled, empty, no-match, and truncated states. Hide
only after a successful launch response. Clear frontend catalog caches on hide
and disconnect, and backend snapshots on Stop. Store no launch history or app
output. Apps is the sixth section in the existing two-row navigation.

## Verification and limits

Go tests cover precedence/visibility, localized metadata, regular/nonregular
files, malformed/oversized input, cancellation, stable revisions, cloned
snapshots, frame bounds, stale IDs, exact GIO arguments, launch failures,
application lifetime independence, and RPC grants/deadlines/validation. QML
tests cover search, button/Enter, busy/denied states, stable row focus, cache
disposal, and accepted-only success signals.

`make e2e-apps` uses an isolated data directory and nested Hyprland session. It
checks real GIO quoting and field codes, launches synthetic GUI apps from the
real Apps page using both the button and Enter, keeps failed launches open,
hides after acceptance, and rejects invalid/removed entries. Host applications
are not launched. The existing private clipboard/favorite lifecycle checks run
in the same fixture.

Icons, launch history/favorites, desktop actions, file/URI opening, fuzzy ranking,
and a unified apps/windows search are deferred. Terminal and D-Bus applications
depend on the desktop's GIO setup; this increment does not install a terminal or
change activation settings. No upstream application source or assets were copied.
