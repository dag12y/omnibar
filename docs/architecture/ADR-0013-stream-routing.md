# ADR-0013: Playback-stream output routing

Status: Accepted

## Decision

Extend the existing `audio` module and `audio.control` permission with
`audio.moveStream`, params `{"key":"stream-key","output":"sink-name"}`.
The result is the same revisioned audio snapshot used by other audio methods.
The driver uses `pactl move-sink-input STREAM_ID SINK_NAME`, as described in
[the pactl manual](https://man.archlinux.org/man/pactl.1).

Before issuing a move, read current server state and resolve both the stream key
and destination name. Missing, malformed, disabled, and denied requests fail
before mutation. A disappeared stream/device or server command failure returns
an error; the UI restores the last confirmed selection and can refresh. External
changes can still race between validation and the command, so server errors are
preserved rather than assuming the requested move succeeded.

Moves target one playback stream. They do not issue commands to change the
system default, volume, mute, or other streams. An app with multiple streams
continues to have independently routed rows. Omnibar stores no routing preference
and does not automatically reapply a route when a stream restarts. WirePlumber
or another session manager may retain preferences or choose fallback devices
according to its own policy. Physical Bluetooth/lid/profile transitions remain
hardware-verification work.

## UI

Each playback row has an OUTPUT selector below volume. The DEFAULT OUTPUT
selector remains separate. Both share `OutputPicker.qml` for styling and
keyboard behavior. Device options use a stable model across ordinary snapshot
refreshes; actual changes to the device list close an open popup to avoid
activating an obsolete option index. Device changes never themselves send a
routing request. An unmatched current destination displays "Output unavailable".
Pending actions use the existing serialized request queue. Completion or failure
resynchronizes selection with server-confirmed state.

## Verification

Go tests cover fresh target validation, permission/parameter rejection, routing
command arguments, and preservation of default/other stream/volume/mute state.
QML tests cover keyboard routing, server confirmation, failure rollback, popup
stability on refresh, and missing-device behavior. The isolated audio end-to-end
test moves a muted test stream while a second stream plays on another virtual
output, checks unrelated state is unchanged, and rejects removed-device and
ended-stream requests. The user's audio stack is not used by that test.
