# ADR-0018: Clipboard Transform & copy

Status: Accepted

## Decision

Extend the existing clipboard module with explicit Transform & copy actions on
recent entries and favorites. Resolve source text by session ID on the backend;
never accept replacement text or executable expressions from the UI. Transform
in memory and copy through the existing driver. No history text or persisted
favorite is edited, no collection toggle is changed, and no paste is injected.

Support four fixed operations:

| Operation | Behavior |
| --- | --- |
| `trim` | Remove Unicode whitespace only at the start and end; keep interior spacing and breaks |
| `lowercase` | Go's locale-independent Unicode lowercase mapping |
| `uppercase` | Go's locale-independent Unicode uppercase mapping |
| `remove_line_breaks` | Replace CRLF, LF, CR, Unicode line/paragraph separators with spaces; CRLF counts as one break |

Line-break removal preserves other spacing and tabs; blank lines can become
multiple spaces. Case conversion uses simple Unicode mappings, without
locale-specific or multi-character case folding. Validate transformed UTF-8
against the existing nonempty/16-KiB item rules, including after case conversion
(which can increase UTF-8 byte length). Reject empty or oversized results before
calling the driver, preserving the current clipboard. Unchanged results may be
copied normally.

The collector follows its existing behavior: when active it may capture a
transformed selection as a recent entry, deduplicate an equal value, and apply
normal eviction limits. Source text is never rewritten. Paused collection stays
paused and adds no new entries. Saved favorites are never automatically replaced
or persisted with transformed text; save the new recent entry explicitly if
wanted. Normal recopying can refresh an entry's timestamp.

## Protocol and permissions

Add `clipboard.transformCopy` with params
`{"id":"1","operation":"uppercase"}`. Require an enabled module,
`clipboard.read`, and `clipboard.write`, before source lookup or clipboard access.
Missing/malformed params and unknown operations return Invalid params. Stale IDs,
empty/oversized results, and copy-driver failures return an action error. Error
messages contain no source text. Return the confirmed clipboard snapshot with
`can_copy`, using the existing three-second RPC deadline and serialized copy
path. Existing `clipboard.copy` keeps its exact-text behavior.

## UI

Each entry has Transform & copy, which expands a two-column set of four actions.
Only one entry's controls are expanded at a time. Labels and descriptions explain
trim edges and replacing breaks with spaces. The panel states that source text
stays unchanged. Require the write grant, ready state, and no pending request to
activate transform actions. Keep actions keyboard accessible and retain stable
row/focus state during refresh. Clear the expanded selection when its source
leaves the snapshot (including frontend cache disposal). Existing controller
errors remain visible across polling, including an empty trim or failed copy.

No new dependencies, persistent format, capability, or configuration is added.
This is the first clipboard-utilities increment; previews, operation chaining,
custom expressions, URL/JSON tools, and locale-sensitive casing are deferred.

## Verification

Go race tests cover Unicode case mapping, whitespace boundaries, CRLF/separator
handling, empty results, byte expansion, stale/unknown actions, permission and
parameter denial, driver failures, unchanged source/revision/favorite storage,
backend-owned source resolution, and RPC deadlines. The complete QML suite has
112 tests, including operation signals, read-only/busy controls, keyboard actions,
focus stability, and source removal.

The private nested Wayland smoke test checks all four real copy outputs, paused
snapshot and favorite-file preservation, active collection of a transformed
result, and empty/unknown operations leaving the clipboard unchanged. The shelf
and favorite lifecycle checks continue to run in the same private session. An
offscreen render verifies the expanded controls fit the panel. No host clipboard
or user favorites are accessed by these tests. No upstream Vorssaint source or
assets were used.
