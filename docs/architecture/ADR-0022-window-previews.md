# ADR-0022: Optional in-memory window previews

Status: Accepted

## Decision

M27 adds an explicit Preview button to running-window rows in Windows and
Search. It captures one thumbnail at a time after a click or keyboard button
activation. Opening a page, changing selection, refreshing metadata, and
reopening the panel do not capture frames. Installed-app results have no Preview
action. A close control discards the current preview; Preview again refreshes it.

Both the existing `window.manage` and `screen.capture` grants and an enabled
window_switcher module are required. The module declares both capabilities, but
metadata and focus RPCs still require only window.manage. Add `previewsAllowed`
to metadata/focus snapshots so the UI can expose the permission state. Example
screen.capture remains false. Reject denied capture requests before querying the
compositor or launching a tool. No new module or separate permission is added.

## Capture identity and protocol

`windows.getPreview {"address":"0x123abc"}` validates a strict window address
against a refreshed bounded list. Resolve its numeric Hyprland `stableId` from
eligible live clients, then call `grim -T IDENTIFIER -s 0.25 -t png -` as exact
arguments without a shell. No title/class matching, output capture, region
cropping, focusing, or workspace moves are used. Missing identifiers, unsupported
capture protocols, missing/old grim, and capture failure are explicit errors.

Use the same explicit HYPRLAND_INSTANCE_SIGNATURE for metadata and capture.
Resolve its Wayland socket using `hyprctl -j instances` in XDG_RUNTIME_DIR;
override the child's WAYLAND_DISPLAY with that exact socket and omit
WAYLAND_SOCKET. This prevents an inherited display or file descriptor from
selecting another compositor. Re-read eligible clients after capture and
discard the image if the address disappeared or its stable ID changed.
Address reuse before the fresh lookup remains an ordinary desktop-action race,
not an isolated identity/session guarantee.

This uses grim's [foreign-toplevel capture option](https://man.archlinux.org/man/extra/grim/grim.1.en)
and the compositor's [foreign toplevel identifiers](https://gitlab.freedesktop.org/wayland/wayland-protocols/-/blob/main/staging/ext-foreign-toplevel-list/ext-foreign-toplevel-list-v1.xml).
Verified with grim 1.5.0 and Hyprland 0.56.2, including off-workspace capture.
`grim>=1.5.0` is an optional runtime dependency; ordinary switching works
without it. No full-screen fallback is attempted.

## Bounds, data, and lifecycle

The RPC has a three-second deadline. Serialize captures with a nonblocking
single-capture gate; capture does not hold the window module's snapshot/focus
mutex. Cap command output at 2 MiB, decoded input at 4096 per dimension and
4,194,304 pixels, and the result at 320×200 without upscaling. Normalize PNG
input to JPEG at quality 75, at most 128 KiB, and return only `address`,
`dataURL`, `width`, and `height`. Metadata and image responses fit the existing
1 MiB IPC frame limit. The subprocess output wrapper deliberately does not
embed bytes.Buffer: its promoted ReadFrom would bypass Write during io.Copy.

Images travel through stdout and local IPC only. There is no image file,
backend preview cache, history, clipboard write, or saved image. Tool stderr is
discarded; errors contain no captured bytes, titles, or compositor diagnostics.

The frontend holds one data URL and uses an Image with cache disabled. It starts
requests only while Windows/Search is visible and authorized. Hiding the panel,
changing pages, disconnecting, losing permission, or removing the target clears
the image and invalidates pending IDs. An already issued one-shot request can
finish within its deadline after hiding; its late response is discarded. There
is no background capture loop or automatic recapture. This does not claim secure
memory erasure or instant cancellation of an in-flight compositor request.

Successful capture keeps the panel open and scrolls the thumbnail into view.
Capture failures show an error in the preview area, preserving switching and
launching. Existing confirmed-focus and accepted-launch signals still dismiss
the panel. Previews are snapshots, not live video streams.

## Verification

Go tests cover independent grants, denial before reads/capture, exact arguments
and compositor socket, address/stable-ID changes, timeout/output bounds, malformed
and large images, thumbnail pixels/aspect ratio/frame bounds, serialized capture,
and focus access during capture. QML tests cover explicit-only requests, permission
gates, one in-flight request, stale/oversized replies, late frames after hiding,
target/page/connection cleanup, and buttons that do not Switch or Launch.

`make e2e-previews` uses synthetic windows in a private compositor. It verifies
JPEG pixels from an off-workspace target without changing focus, real Windows
and Search Preview buttons, rendered image readiness, no automatic captures on
page/hide/reopen, and invalid/closed targets. ImageMagick is used only by this
optional smoke test to check decoded pixels. `--capture` saves explicitly
requested test UI screenshots for layout inspection, not production previews.

Protected/unmapped/special-workspace windows, legacy compositors without exact
capture support, oversized captures, live video, multi-window thumbnail grids,
and persistent screenshots remain outside this increment. Development stays on
0.2.0 with local package release 3; no public release is implied.
