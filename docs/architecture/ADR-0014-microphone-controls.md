# ADR-0014: Microphone controls and recording-app metadata

Status: Accepted

## Decision

Extend the existing `audio` module under `audio.control`. Read `pactl` source
and source-output metadata alongside playback state. Add device gain, mute, and
default-input commands, using the same live validation, deadlines, serialized
mutations, revisioned snapshots, and demand-driven updates as playback.

Gain is the source's volume setting from 0 to 100 percent, not a sound-level
meter. Omnibar opens no capture stream, reads no microphone samples, and stores
no recordings. Recording apps are source-output metadata reported by the audio
server: an open or paused stream is not proof that an app is saving audio, and
clients bypassing that server may not be represented.

Speaker-monitor sources remain in snapshot metadata so recording streams can
be labeled accurately. They are excluded from the microphone selector and
rejected by microphone mutation methods. Identify monitors using `device.class`,
the conventional `.monitor` name suffix, or an explicit monitor-of-sink index.
Non-monitor virtual sources are supported alongside physical microphones.

## Protocol

Add `default_input`, `inputs`, and `recordings` to audio snapshots. These arrays
are empty when the audio service is unavailable. Input entries include `id`,
`name`, `description`, `volume`, `muted`, and `monitor`. Recording entries include
app/media/icon identity, `input_id`, and `paused`. Existing playback fields and
methods remain compatible.

| Method | Params | Result |
| --- | --- | --- |
| `audio.setInputVolume` | `{"input":"source-name","volume":55}` | Snapshot after gain change |
| `audio.setInputMute` | `{"input":"source-name","muted":true}` | Snapshot after mute change |
| `audio.setDefaultInput` | `{"input":"source-name"}` | Snapshot after default change |

All use the existing capability, including metadata reads/subscriptions. A missing
or disabled grant is rejected before contacting the service. Empty/malformed
params and levels outside 0–100 are rejected. A named input must still exist and
must not be an output monitor. No command implicitly unmutes a device, changes
its other settings, or explicitly reroutes a recording app. Default-input changes
may affect existing clients according to session-manager policy.

The adapter uses `set-source-volume`, `set-source-mute`, and `set-default-source`;
see [pactl](https://man.archlinux.org/man/pactl.1). Source/source-output events now
trigger refreshes. Snapshot reads consist of info, sinks, sink-inputs, sources,
and source-outputs. The watcher/resync stops when the Audio page has no clients.

## UI

Audio has Playback and Microphone views. Microphone shows the default-input
selector, that microphone's gain/mute, and recording-app source labels. System
audio captures are labeled separately from microphones. Missing devices and
unavailable/cached metadata are explicit. The view loads lazily. Input selection
and opening the view never open a capture stream or change gain/mute by themselves.

`DevicePicker` shares the existing stable-model keyboard/hotplug behavior between
microphone and output selectors; `OutputPicker` remains a playback wrapper. Gain
reuses `MixerRow` with routing hidden. Pending gain/mute commands are coalesced
per input name so commands for different microphones cannot overwrite each other.
The panel IPC action `showMicrophone` opens this view directly, for shortcuts.

## Verification and limits

Go tests verify source/monitor parsing, recording source identity, permission and
parameter denial, stale-device rejection, and preservation of unrelated playback
and microphone settings. QML tests verify gain/mute, default changes, monitor
exclusion, source labels, missing devices, unavailable controls, and request queues.
The opt-in audio smoke test uses FIFO-backed virtual sources in an isolated audio
stack, a recorder that discards virtual data, input removal, and real UI loading.
Opening the view is checked to leave settings and recording-stream IDs unchanged.
The normal user's microphone and playback services are untouched.

Per-recording-app input routing, recording-stream volume/mute, live sound-level
meters, noise suppression, and global all-microphones mute are deferred. Physical
Bluetooth profile/reconnection and hardware mute-switch behavior need hardware
verification. Omnibar stores no input preferences; persistence belongs to the
server/session manager.
