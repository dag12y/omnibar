# ADR-0015: Session clipboard history

Status: Accepted; favorite persistence and Clear semantics extended by
[ADR-0016](ADR-0016-clipboard-favorites.md).

## Decision

Implement text history in the static Go `clipboard` module using
[wl-clipboard](https://github.com/bugaevc/wl-clipboard). Keep entries in memory;
SQLite is unnecessary for this session-only increment. Every backend session
starts paused. Granting `clipboard.read` permits access but does not start a
watcher; the user explicitly selects Start history. Starting can collect the
current selection. Collection continues while the panel is hidden until Pause
or backend shutdown. Pause cancels and reaps the watcher and capture helpers.

Store at most 50 entries, 16 KiB per item, and 128 KiB of text total, evicting the
oldest first. Exact duplicates move to newest with the same session ID. Accept
valid UTF-8 with newlines, carriage returns, and tabs; reject empty, oversized,
binary/control data. No images, files, clipboard persistence, or content logs.
Dropping references does not guarantee secure memory erasure.

`wl-paste --type text --watch` runs the current executable in internal
`--clipboard-capture` mode. This helper reads at most 16 KiB + 1 byte with a
2-second timeout and emits one bounded JSON frame through a private stdout pipe.
Only the `data` state is read. Sensitive, clear, nil, and unknown states are
skipped. See [the upstream watcher](https://github.com/bugaevc/wl-clipboard/blob/master/src/wl-paste.c)
for the state convention. Sensitive hints depend on the source app; unmarked
passwords and other secrets can still enter history. This is not a password
classifier. Watcher failures retry every two seconds until paused.

Copy uses `wl-copy` with exact text over stdin and a three-second RPC deadline.
It changes the clipboard selection without injecting keystrokes. Its clipboard
owner can survive backend exit, preserving the current selection separately
from the forgotten history. Clear/Delete affect history only. No clipboard text
is passed as command arguments or printed to diagnostic output.

## Protocol and permissions

All methods require an enabled module and `clipboard.read`. Only `clipboard.copy`
also requires `clipboard.write`. Both grants remain false in the example config.
Read-only collection, inspection, deletion, and clearing are supported.

| Method | Params | Result |
| --- | --- | --- |
| `clipboard.getSnapshot` | `{}` | Current snapshot |
| `clipboard.setCollecting` | `{"collecting":true}` | Snapshot after Start/Pause |
| `clipboard.copy` | `{"id":"1"}` | Snapshot after copy-back |
| `clipboard.delete` | `{"id":"1"}` | Snapshot after deletion |
| `clipboard.clear` | `{}` | Snapshot after clearing |

Snapshots contain `revision`, `collecting`, `state` (paused/watching/retrying),
optional `error`, `can_copy`, and `entries` (`id`, `text`, `copied_at`). Entries
are newest first. IDs are monotonic within the backend session. Reject malformed
or missing action params and stale IDs. Mutations/lifecycle changes serialize;
watch ingestion and snapshots share a lock. Snapshot size remains below the IPC
frame limit even with JSON escaping at the text byte cap.

The Clipboard controller polls snapshots once per second only while visible.
It uses no content events or generic module subscription, whose all-capabilities
policy would otherwise prevent read-only use. Serialize actions, skip polling
while busy, reconcile confirmed revisioned snapshots, and clear frontend caches
and pending queues on hide/disconnect. Backend collection follows its explicit
toggle rather than subscription demand. The UI supports local case-insensitive
search, plain-text previews, and stable-ID rows to preserve keyboard focus.
`panel showClipboard` opens the page directly.

## Verification and limits

Go tests cover bounds, escaped frame size, UTF-8 rejection, hints without reading
stdin, deduplication, exact copy, pause/reaping, shutdown cleanup, and independent
grants. QML tests cover explicit collection, search/actions, read-only controls,
focus stability, cache clearing, and permission failures. `make e2e-clipboard`
starts a private nested Hyprland compositor and isolated backend/config, using
synthetic selections only. It verifies real collection/copy, sensitive hints,
pause, delete/clear, hidden-panel behavior, and a clean paused restart. It does
not replace or read the host clipboard. The local package adds `wl-clipboard`.

Persistent history, favorites, image/file capture, automatic paste, per-app
exclusions, and primary-selection history remain deferred. Other Wayland
compositors require data-control support and separate compatibility testing.
