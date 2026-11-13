# ADR-0021: Unified app and window search

Status: Accepted

## Decision

M26 adds a Search page that combines the existing installed-app catalog and
running-window snapshots. Open it using the panel header's Search button,
Ctrl+K while the panel has keyboard focus, or `panel showSearch` over Quickshell
IPC. The dedicated Apps and Windows pages remain available. No host keybinding
is installed and no arbitrary-command interface is added.

Reuse the existing AppController and WindowController and their independent
permission gates (`process.manage` and `window.manage`). Both are active only
when their dedicated page or Search is open. Disabled, denied, offline, loading,
and failed sources show their own status; one usable source is enough to search
and act. A global busy guard prevents overlapping actions through this page.
Backend target revalidation and deadlines remain unchanged.

## Results and actions

Match every whitespace-separated token, case-insensitively, across window
title/class/workspace or app name/description/keywords. Running matches come
before installed matches. Within each kind, rank exact, prefix, name substring,
and secondary-field matches, then sort by title and stable identity. An app and
a window with the same display name remain distinct choices. No class/name
heuristic silently merges them or decides which operation to execute.

Render at most 30 results from the bounded source catalogs (100 windows,
200 apps). Display a notice when result or source limits apply. Typing searches
all entries in the available source snapshots before applying the result cap.
There is no fuzzy/MRU ranking, additional disk scan, or new backend module.

Each result shows WINDOW or APP and an explicit Switch or Launch button.
Up/Down in search changes the highlighted result; Enter activates it. Keep
keyboard selections and address/desktop-ID keyed rows stable through polling.
Before the user selects a row, late source replies may update the default to
the first running match. New query text resets to the first match; a removed
selection falls back to the first surviving result. Empty results do nothing.
The search field receives focus once a source becomes ready. Ctrl+K can also
return focus to search and select its text while already on the page.

The highlighted result scrolls into view during keyboard navigation. Typing
returns the search field to view. The panel's existing screen-height cap and
internal scrolling remain in force. Render titles/descriptions as plain text.

Dispatch only the selected result's exact identity to `windows.focus` or
`apps.launch`. Existing controller success signals hide the panel: focus must
be confirmed by Hyprland, while GIO launch acceptance does not guarantee a new
window. Failures remain visible on the page, with the originating source's
error. No launching fallback is attempted after a failed Switch.

## Lifecycle and privacy

Hiding Search or closing the panel clears its query, selection, and result
model. Disconnection drops both source snapshots and displayed results. Each
hidden dedicated page receives an empty snapshot while Search owns the shared
controllers, avoiding duplicate hidden title/catalog models. Existing backend
window and app metadata remains bounded in memory; no search or action history
is written. No new permission or runtime dependency is introduced.

## Verification

QML tests cover multi-token matching and ranking, distinct identities, Switch
versus Launch dispatch, buttons/Enter/arrows, busy guards, independent source
permissions, late replies, stable row focus/selection, query reset, result caps,
and delayed keyboard focus. Existing controller tests continue to cover
confirmed/accepted-only success signaling and frontend cache disposal.

`make e2e-search` uses isolated desktop entries and a nested Hyprland instance.
It opens Search with Ctrl+K, retains failed launches, uses arrows and Enter to
launch a synthetic GUI app, switches to the expected running target, verifies
panel dismissal and reopen reset, and runs existing private Apps/clipboard
regressions. `--capture` saves both the initial page and a scrolled selection
for inspection. Host applications are not launched or focused by this test.

Package lifecycle verification compares all shipped file contents and modes,
including removing added Search files on downgrade to M25. This supports UI-only
updates where the backend binary stays identical.

The increment targets the existing 0.2.0 development version. The local package
release becomes 2 to distinguish it from M25; no new public release is implied.
