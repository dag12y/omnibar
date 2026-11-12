# ADR-0016: Explicitly saved clipboard favorites

Status: Accepted

## Decision

Extend [session clipboard history](ADR-0015-clipboard-history.md) with explicit
Save favorite/Unsave operations. Only selected favorites persist. Ordinary
collection remains session-only and starts paused after every backend restart.
Restoring favorites does not start the watcher or change the system clipboard.

Keep favorites in a versioned, bounded JSON file rather than introducing SQLite
for a list of at most 20 saved items and 32 KiB of text. This narrows the earlier
storage plan: SQLite remains deferred for features needing queries or richer
persistent data. The file is `$XDG_DATA_HOME/omnibar/clipboard-favorites.json`,
falling back to `~/.local/share/omnibar/clipboard-favorites.json`. Reject relative
XDG data paths. Store exact text and the copy timestamp, without session IDs.
Generate fresh IDs when restoring. A repeated copy preserves favorite status
and its current session ID; a later save may refresh persisted copy timestamps.

Favorites count within the shared 50-entry/128-KiB list budget and retain the
16-KiB item limit. Eviction removes the oldest non-favorite entries. Snapshot
ordering places favorites first, then recent copies; ordering within each group
follows recency. Clear now means Clear recent and preserves favorites. Unsave
removes the disk copy but leaves a recent entry; Remove deletes a favorite from
both disk and list. Neither operation changes the clipboard selection.

## Storage and failures

Load favorites only when the clipboard module is enabled and `clipboard.read`
is granted. Missing files mean no favorites. Restore is bounded to 256 KiB of
encoded JSON; validate version, exact schema, text, timestamps, deduplication,
count, and text byte limits. Refuse symlink, non-regular, and group/other-readable
files. Invalid data stays untouched and disables saved-item mutations until
repaired and the backend restarted, while recent history remains usable.

On explicit favorite changes, write only favorites to a mode-0600 temporary file
in the mode-0700 Omnibar data directory, sync/close it, then atomically rename it
over the target. Serialize mutations and ingestion during this bounded write.
Change visible state only after success. Failed writes preserve both confirmed
state and the previous saved file; remove temporary files on handled failures.
A crash before rename may leave a private temporary file. This is unencrypted
local text storage, with no secure-erasure or full power-loss durability guarantee.
Content never appears in diagnostic output or command arguments. Package
upgrades/removal preserve user data, and backups may contain saved text.

## Protocol and UI

Add `clipboard.setFavorite` with params `{"id":"1","favorite":true}`. Require
an existing ID and a boolean; return the confirmed snapshot. All existing
clipboard methods still work. The existing `clipboard.read` grant permits
managing favorites; only copy-back also requires `clipboard.write`. No new grant
or automatic persistence of collected content is introduced.

Snapshots add `favorite` to each entry, `can_favorite` for storage availability,
and optional `favorites_error`. Existing bounds still keep fully escaped
snapshots below the 1-MiB IPC frame limit. Clear returns retained favorites.
Delete on a saved entry commits disk removal before removing it from the list.

The UI displays Save favorite (explicitly described as writing to disk), Unsave,
Remove, a saved badge, and All copies/Favorites filters with a count. Search
applies within either filter. Clear recent is disabled when no recent entries
exist. Save controls reflect storage availability. Action failures remain
visible across ordinary polling until another action; corrupt-file errors are
shown independently of watcher state. Stable IDs preserve row focus while
reordering. Hiding/disconnecting still clears frontend content caches.

## Verification

Go tests cover explicit-only persistence, exact restore, private permissions,
protected eviction, clear semantics, duplicate copies, unsave/delete, limits,
frame size, stale/invalid parameters, read-only management, save-failure rollback,
corrupt-file preservation, and unsafe-file rejection. The 94-test QML suite covers
filtering/actions, read-only copy, clear state, storage availability, focus, and
persistent action-error display.

The nested Wayland smoke test uses a private XDG data home and synthetic text.
It verifies private file modes, Clear recent preserving favorites, paused restart
restoration without ordinary history, exact copy, and persisted removal. Normal
clipboard selections and user favorites are untouched. No upstream Vorssaint
source or assets were used. Full persistent history, favorite editing/reordering,
import/export, image/file favorites, and encryption remain deferred.
