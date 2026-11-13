# ADR-0019: Hyprland running-window switcher

Status: Accepted

## Decision

Add a static `window_switcher` module and Windows page for searching and focusing
existing Hyprland clients. Application launching remains deferred. This increment
introduces no window moves, layouts, resizing, closing, preview capture, or
persistent window history.

Use `hyprctl -j clients` and `hyprctl -j activewindow` on demand. Require the
backend's `HYPRLAND_INSTANCE_SIGNATURE` to be present rather than allowing
hyprctl to choose another compositor implicitly. Missing tools, sessions, malformed
replies, or command failure produce a generic unavailable error without title
contents. Command stdout is capped at 2 MiB and stderr is discarded.

Include mapped, non-hidden, input-capable clients across ordinary workspaces.
Ignore special-workspace clients and clients explicitly rejecting input. For
older metadata lacking acceptsInput, mapped/non-hidden clients remain eligible.
Use initialClass when class is empty, then generic app/title fallbacks. Strip
control characters, repair invalid UTF-8, and bound title/class/workspace labels
to 512/128/64 bytes. Render plain text. Sort by app class, title, then address for
stable refreshes. Show at most 100 windows with an explicit truncation notice;
confirmed snapshots remain well below the IPC frame cap.

## Focus and permissions

Both methods require an enabled module and the existing `window.manage` grant,
which remains false in the example config. Reject access before querying the
compositor. Startup does not read windows or allocate a watcher. No other
capability or new required package is introduced; Hyprland is an optional package
dependency providing hyprctl.

| Method | Params | Result |
| --- | --- | --- |
| `windows.getSnapshot` | `{}` | Refreshed running-window snapshot |
| `windows.focus` | `{"address":"0x123abc"}` | Snapshot with confirmed active target |

Snapshots contain `revision`, `truncated`, and `windows` with `address`, `title`,
`class`, `workspace`, `monitor`, and `active`. Require strict `0x` plus 1–16 hex
digits for target addresses. Reject arbitrary class/title selectors and shell
syntax. Refresh eligible live windows before dispatch, and require the address
to be present in the currently bounded list. Dispatch an exact `hyprctl dispatch focuswindow address:...` selector. If
legacy dispatch fails, retry with the fixed Lua dispatcher expression
`hl.dsp.focus({ window = "address:..." })` for Lua-configured sessions. Only
the strictly validated hexadecimal address is interpolated; neither command
uses a shell or batch.
Refresh afterward and return success only if the target is reported active.
Closed windows, failed dispatch, or unconfirmed focus remain errors. A window
can still disappear or an address can be reused between these checks; this is
an OS desktop action, not an isolated identity/session guarantee.

RPC calls share a three-second deadline and serialized snapshot/focus access.
Hyprland owns workspace switching and focus policy. The adapter's exact-address
selector follows the [Hyprland selectors documentation](https://wiki.hypr.land/configuring/selectors/);
both legacy and Lua focus dispatch are supported on Hyprland 0.56.2.
Unsupported compositor replies fail explicitly.

## UI and lifecycle

Windows provides app/title/workspace search, workspace/monitor labels, active
badges, Switch buttons, and Enter to switch to the first search result. It reads
only while visible, polling every 1.5 seconds through the shared SnapshotController.
Use stable address-keyed rows to preserve keyboard focus through refresh.
Unavailable/denied/busy states disable actions. Confirmed windows.focus success
emits a dedicated signal to hide the panel; failures keep it open with the error.
Hiding/disconnecting clears frontend title caches. Backend metadata is bounded
in memory until shutdown; no title/history file or content log is written.

`panel showWindows` opens the page directly for shortcuts. Navigation uses two
rows to keep all five sections usable at the existing panel width. Generic
module subscriptions skip this module; snapshot RPCs drive its demand. No host
configuration/keybindings are changed automatically.

## Verification and limits

Go race tests cover parser filtering/fallbacks, sorting, stable revisions,
snapshot cloning, UTF-8/label/window/output/frame bounds, exact selectors,
closed targets, missing sessions, dispatch errors, confirmation failure,
shutdown disposal, grants/params, and deadlines. The complete QML suite has
123 tests, including search/Enter/actions, busy/denied/empty state, stable row
focus, confirmed-only success signaling, and frontend cache disposal. An
offscreen synthetic render checks page layout.

`make e2e-windows` uses a private nested compositor and explicitly assigns its
instance signature before starting the backend. Two synthetic Qt windows verify
real listing, exact focus across workspaces, active-window confirmation, the
real Windows page, stale/invalid targets, and empty desktop recovery. It does
not list or focus host windows. Clipboard/favorite regression checks continue
in that private session. Application launching, special-workspace restoration,
group-tab handling, MRU ordering, screenshots/previews, and non-Hyprland
compositors remain deferred. No upstream Vorssaint source or assets were copied.
