# ADR-0023: Exact window controls

Status: Accepted

M28 extends Windows with an expandable Controls area: move to an absolute
Hyprland workspace ID (1–9999), set floating/tiled state, and maximize/restore.
All operations require window.manage and an enabled window_switcher module.
The panel stays open after changes and displays confirmed metadata, not
optimistic switch states. Ordinary Switch remains the only window action that
dismisses the panel. No desktop keybindings/config are changed.

RPCs are `windows.moveWorkspace {address,workspace}`, `windows.setFloating
{address,enabled}`, and `windows.setMaximized {address,enabled}`. Missing booleans,
wrong types, unsafe addresses, and invalid IDs fail before compositor access.
Validate the target against a fresh bounded list and reject pinned/grouped
windows. Require users to exit full screen before changing controls, and to
restore maximization before changing floating mode. Desired states are
idempotent. Re-read after dispatch, retrying confirmation briefly within the
existing three-second deadline; unconfirmed changes remain errors.

Use exact address selectors for legacy move/float commands and the native Lua
dispatcher fallback. Move uses follow=false. Maximization uses only the Lua
dispatcher with its explicit window selector: the legacy fullscreen command
acts on the focused window and is deliberately never attempted. Unsupported
legacy configurations report an error rather than changing a different window.
API syntax was checked against installed Omarchy bindings and
[Hyprland dispatchers](https://wiki.hypr.land/0.57.0/configuring/core/dispatchers/).

Snapshots add workspace ID, floating/fullscreen state, restricted status, and
geometry for subsequent saved-layout support. Geometry dispatch is internal to
the adapter and bounded; no general command/expression RPC is exposed. Existing
metadata, previews, permission gates, and confirmation rules remain in force.

Go tests cover exact selectors, validation, grants, idempotence, restrictions,
unrelated targets, driver failures, and failed confirmation. QML tests cover
desired-state parameters and busy/fullscreen guards. `make e2e-controls` runs a
private Lua compositor, changes a background fixture, verifies unrelated state,
and activates the real Float control. Host windows are not changed by the test.
