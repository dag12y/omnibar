# ADR-0025: Local plain-text Scratchpad

Status: Accepted for M30, v0.3.0 development.

## Decision

Add a `notes` module gated by `notes.manage`. This grants access only to
Omnibar-owned note data, not arbitrary filesystem paths. `notes.copy` additionally
requires clipboard.write. Disabled/denied modules do not load note files.

Store versioned JSON at `$XDG_DATA_HOME/omnibar/notes.json` (default
`~/.local/share/omnibar/notes.json`), privately and atomically. Reject symlinks,
nonregular/insecure files, malformed data, relative XDG roots, duplicate IDs,
and oversized data; preserve originals and report errors. Limits: 100 notes,
32 KiB UTF-8 body per note, 128-byte control-free nonblank title, 2 MiB encoded
file. Bodies are plain UTF-8, allow tabs/newlines, and reject NUL. No content logs,
external commands, network, rich text, imports, or attachments.

`notes.getSnapshot` returns revision and note summaries (ID, title, pin,
revision), sorted pinned first then most recently changed. `notes.get` returns
one full note. `notes.create`, `notes.update`, `notes.delete`, and `notes.copy`
use stable opaque IDs. Updates/deletes/copies require the expected per-note
revision. Each successful mutation increments the durable global revision and
sets the changed note's revision to that value. A stale revision fails without
writing. Failed persistence leaves memory unchanged. Responses expose the
snapshot and, for create/get/update, the chosen note. Snapshot search uses
bounded body matching in the backend and returns no body text. No module events
contain note content.

The editor debounces autosave and distinguishes Saving, Saved, and Unsaved.
Keep edits in memory across page hide, reconnect, and save failure; flush on
hide when connected. Never replace a dirty draft with a polling response. A
stale edit requires explicit reload/discard or saving as a new note. If an
acknowledgment is lost, compare the saved note with the draft on retry before
attempting another update. Unsaved drafts cannot survive a UI process crash;
state that limit and never label unacknowledged edits Saved. Confirm deletion.
Clipboard copy uses the saved backend body, never an arbitrary frontend payload.

## Verification

Go/RPC checks cover permission gates, revision conflicts, persistence rollback,
restart, limits, private files and corrupt data. QML checks cover debouncing,
late saves, dirty drafts on hide/disconnect, conflicts, and selection stability.
A private compositor test edits a real note, verifies autosave/reopen/restart,
copies exact text, pins/searches/deletes, and checks denied/corrupt states.
Package lifecycle checks preserve note data across transactions. No host note
content or clipboard text is read for verification.

The editor uses [Qt KeyNavigation](https://doc.qt.io/qt-6/qml-qtquick-keynavigation.html)
with BeforeItem priority so Tab leaves the text area for its actions.
