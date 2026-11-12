# ADR-0017: Session file shelf

Status: Accepted

## Decision

Add a static Go `file_shelf` module and a Files page. The first increment keeps
references to explicitly selected local files and directories in memory. It
reads metadata only, without reading contents, recursively scanning folders,
creating copies, moving, renaming, or deleting originals. Shelf references clear
on backend restart. Persistence, transfer operations, and drag-out are deferred.

Accept absolute paths or local `file:` URLs, including percent-encoded spaces,
Unicode, and filename punctuation. Reject remote hosts except `localhost`,
queries/fragments, relative paths, control characters, invalid UTF-8, special
files, and missing targets at add time. Follow symlinks for metadata but retain
the supplied clean path; deduplication is lexical, not inode-based. The shelf
tracks the path rather than an inode: replacing a file at that path updates its
metadata. Broken or unsupported targets become unavailable and can be removed.

Validate the entire batch before changing state. Re-adding a path keeps its
session ID and moves it to the front. Keep supplied order within a batch. Limit
the shelf to 50 entries, 4096 bytes per path, and 32 KiB total normalized path
bytes. Reject a full shelf without evicting references. These limits also bound
escaped snapshot frames below the IPC cap. Metadata refreshes during visible
polling; no filesystem watcher or background directory scan is introduced.

## Permissions and actions

Require an enabled module and the existing `filesystem.access` grant for every
method. Require `clipboard.write` additionally for Copy path. Both grants remain
false in the example config. Merely registering/starting the module touches no
user paths; denied calls return before path validation, metadata, or desktop
helpers. The UI displays the config path and grant needed when access is denied.

| Method | Params | Result |
| --- | --- | --- |
| `shelf.getSnapshot` | `{}` | Refreshed snapshot |
| `shelf.add` | `{"paths":["/absolute/path","file:///local/path"]}` | Snapshot after atomic batch add |
| `shelf.remove` | `{"id":"1"}` | Snapshot after removing one reference |
| `shelf.clear` | `{}` | Empty snapshot |
| `shelf.copyPath` | `{"id":"1"}` | Snapshot after copying an absolute path |
| `shelf.openFolder` | `{"id":"1"}` | Snapshot after requesting the folder opener |

Snapshots contain `revision`, `can_copy`, and `entries`, each with `id`, `path`,
`name`, `directory`, `available`, and `size` (zero for directories/unavailable
items). IDs are monotonic within the backend session. Validate IDs and refresh
target metadata before invoking helpers. Failures preserve shelf references;
unavailable items remain removable. No filesystem or clipboard operation runs
merely from opening the Files page.

Copy path uses `wl-copy` with the exact path on stdin, without a shell. It copies
text, not a transferable file payload. Open folder invokes `xdg-open` with one
absolute directory argument, using the item itself for directories or its parent
for regular files. It does not open the file's contents. Both actions have a
three-second RPC deadline and suppress helper diagnostics that might expose
paths. Opener success means the desktop helper accepted the request; Omnibar
cannot confirm that a particular file-manager window appeared. Paths may change
between validation and desktop dispatch; this is not an OS filesystem sandbox.
`xdg-utils` is an added runtime dependency.

## UI and lifecycle

Files offers a path field, URL drop zone, local search, size/type/status labels,
Open folder, Copy path, Remove, and Clear shelf. Removing/clearing affects only
references. Labels explicitly describe session lifetime and unchanged originals.
[Qt DropArea](https://doc.qt.io/qt-6/qml-qtquick-droparea.html) receives URL drops.
Only [Copy actions](https://doc.qt.io/qt-6/qml-qtquick-dragevent.html) are accepted;
move-only sources are rejected so a successful drop never asks the source to
remove originals. Backend validation remains authoritative; unsupported drops
produce an error without partial insertion. File paths render as plain text.

A shared `SnapshotController` handles serialized requests, revision reconciliation,
timeouts, permission failures, and cache disposal. Clipboard and File shelf use
small wrappers with their own method prefixes and polling intervals. Shelf polls
every two seconds only while visible. Hiding/disconnecting clears frontend caches
but preserves backend references until shutdown. Generic module subscriptions
skip the shelf so read-only filesystem use needs no clipboard-write grant.
Stable row models retain keyboard focus on ordinary refresh/reordering.
`panel showFiles` opens the shelf directly.

## Verification and limits

Go race tests cover batch atomicity, URL/UTF-8 validation, deduplication/IDs,
snapshot copies, count/byte/frame bounds, metadata loss/recovery, special files,
symlinks, untouched originals, driver errors, shutdown clearing, and fail-closed
RPC/action grants. QML tests cover typed paths and URL-batch submission, copy-only
drop handling, search/actions, missing/read-only states, focus, controller
namespaces, errors, and cache clearing. The complete QML suite has 109 tests.

`make e2e-shelf` extends the private nested Wayland clipboard test using synthetic
files. It checks real RPCs/copy-path, recorded opener arguments (without launching
a host file manager), unavailable/recovery, real Files page loading, original
preservation, and session reset. Optional capture saves the nested panel. The
normal desktop clipboard and user files remain untouched. Native file-manager
pointer delivery and specific opener integrations still need manual desktop
verification. No upstream Vorssaint source/assets were copied.
