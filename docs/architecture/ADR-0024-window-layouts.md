# ADR-0024: Explicit saved workspace arrangements

Status: Accepted (M29)

## Decision

Extend the windows module under its existing `window.manage` capability. Save
one ordinary workspace's currently mapped windows as a named arrangement. Store
class, title, workspace ID, floating/maximized state, floating geometry, and a
SHA-256 fingerprint of sorted monitor names, positions, sizes, scales, and
transforms. Window addresses and live toplevel IDs are never persisted.

`windows.saveLayout` takes `{name, workspace}`; saving an existing name
(case-insensitive) replaces it and preserves its opaque ID. Names are trimmed,
control-free UTF-8 of at most 64 bytes. `windows.restoreLayout` and
`windows.deleteLayout` take `{id}`, a 32-character lowercase hexadecimal ID.
Each method returns the ordinary window snapshot. Snapshots add `layouts`
summary rows (`id`, `name`, `workspace`, `count`) and optional `layoutError`.
Saved title metadata is not returned in layout summaries. All three methods
check module availability and permission before storage or compositor work.

Storage is `$XDG_DATA_HOME/omnibar/window-layouts.json`, defaulting to
`~/.local/share/omnibar/window-layouts.json`. Relative XDG roots are rejected.
The versioned JSON is capped at 64 KiB, 8 layouts, and 16 windows per layout.
Writes use a private temporary file, fsync, and atomic rename in a 0700
directory. Existing unsafe/nonregular files and corrupt/oversized data are
preserved; persistence failure leaves in-memory layouts unchanged. Reads use
no-follow/nonblocking flags and require a private regular file. Startup loads
metadata only and does not query or change windows. No automatic save/apply,
search history, image persistence, or application commands are introduced.

## Restore behavior

Before mutation, verify unchanged monitor topology and a complete window list.
Resolve each saved class/title to exactly one current window; class alone is
allowed only when that class has exactly one current window. Refuse reused
matches, missing/ambiguous windows, pinned/grouped/fullscreen targets, and
missing stable toplevel identities. This intentionally requires apps to be
open already. Save also rejects duplicate class/title pairs.

Apply confirmed desired-state controls from ADR-0023: unmaximize, move without
following, set floating/tiling, restore floating geometry, then maximize when
saved. Check the live stable identity again before every step, including
inside the locked control operation. Check monitor topology before each window
and after all actions. Floating geometry uses exact-target resize/move;
maximized floating geometry is left to the compositor. Tiled window order and
split ratios follow the current Hyprland layout rather than a saved tiling tree.

One restore can run at a time, bounded by a 12-second RPC deadline. Restore
releases the snapshot mutex between actions, so ordinary listing remains
responsive. The frontend allows 15 seconds for its response and keeps the panel
open. A changed target or compositor failure stops further steps and reports
possible partial state; this is not a transaction and does not roll back user
changes. The user can fix the cause and explicitly retry. A changed monitor
arrangement requires saving a layout again. Legacy fullscreen dispatch cannot
safely target background windows; targeted maximization requires Lua support.

## Verification

Go tests cover storage permissions, corrupt/symlink preservation, limits,
replacement and rollback, restart without application, matching/topology guards,
partial dispatch failure, responsive reads, and concurrent-restore rejection.
RPC tests cover capability/module gates and parameter validation before driver
work. QML tests cover saved selection across polling, parameters, busy/error
states, and hide cleanup. `make e2e-layouts` verifies geometry/state restoration,
UI Save, replacement, restart, missing targets, and deletion in a private Lua
compositor with private XDG data. Synthetic layouts are included in isolated
package upgrade/downgrade/removal data-preservation checks.
