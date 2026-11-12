# ADR-0012: Playback audio mixer

Status: Accepted

## Decision

Add the static in-process `audio` module, gated by `audio.control`. Use `pactl`
against PipeWire's PulseAudio-compatible service (`pipewire-pulse`) instead of
introducing a native PipeWire binding or audio processing engine. On Arch,
`libpulse` provides `pactl`; `pipewire-pulse` provides the service. PulseAudio's
compatible protocol may also work, but PipeWire on Omarchy is the tested target.

Sources consulted:

- [PipeWire's PulseAudio compatibility](https://github.com/PipeWire/pipewire/blob/master/README.md)
- [pactl manual](https://man.archlinux.org/man/pactl.1)
- Vorssaint `UI/MenuPanel/MixerSection.swift` at the pinned reference commit in
  [the source review](../development/vorssaint-reference.md): app identity,
  volume percentage, mute, and output controls. No upstream code or assets were
  copied into Omnibar.

## Scope and interaction

The Audio page lists playback streams, labeled with app name, icon, media name,
and output. An app with multiple streams has multiple independently adjustable
rows. Apps without an audio stream do not appear; paused streams can remain
listed. A row supports volume from 0 to 100 percent and explicit mute/unmute.
Pointer drags commit on release; keyboard adjustments commit on each step.
Reading a level above 100 percent preserves its actual label, while adjustments
are limited to 100. A scalar volume command applies to all stream channels.

The output selector sets the system default playback device. Existing streams
follow the session manager's policy; this is not explicit per-stream routing.
Per-app routing, microphone controls, boost, master-output volume, persistent
levels, and grouping multiple streams into one app row are deferred.

## Runtime behavior

- `pactl --format=json info`, `list sinks`, and `list sink-inputs` supply state.
- `pactl subscribe` watches sink-input, sink, server, and card changes. Ignore
  client/source events, including connections made by Omnibar's own commands.
- Coalesce notifications for 100 ms and resync every three seconds while needed.
- Start the watcher only while a client subscribes. Cancel and reap it when the
  last subscriber leaves, including on disconnect or page hide. Retry failed
  connections after two seconds while still subscribed.
- Reads have bounded deadlines; actions have a five-second RPC deadline.
  Unavailable audio is a snapshot state, so the module and other utilities keep
  running and reconnect when the service returns.
- Serialize reads/actions, return an increasing revision, and ignore older
  snapshots in the UI. The UI maintains stable delegates by stream key so
  refreshing does not destroy a focused or dragged slider.
- Before an action, read fresh server state and match the requested key/output
  to a current entry. A stream key combines index, object serial, process ID,
  and app name. This reduces stale-index mistakes; it is not a persistent ID or
  a transactional guarantee against an external change between commands.
- Commands use argument vectors, not a shell. No audio is captured, decoded,
  rerouted through Omnibar, or sent over the network by this module.

The Go module is in-process; only the short `pactl` commands and cancellable event
watcher are child processes. This is the initial audio implementation rather
than the separately supervised audio service considered in the broader design.

## Protocol

All methods require `audio.control`, an enabled module, and a running registry
state. Missing grants fail closed before contacting the audio service.

| Method | Params | Result |
| --- | --- | --- |
| `audio.getSnapshot` | `{}` | Current snapshot |
| `audio.setVolume` | `{"key":"…","volume":45}` | Snapshot after change |
| `audio.setMute` | `{"key":"…","muted":true}` | Snapshot after change |
| `audio.setDefaultOutput` | `{"output":"sink-name"}` | Snapshot after change |

A snapshot includes `revision`, `available`, optional `error`, `server`,
`default_output`, `outputs`, and `streams`. Subscribe with `module.subscribe`
for module `audio`; `module.event` notifications use event `snapshot` with the
same payload. A disconnected server returns empty arrays and `available:false`.
Invalid/missing action parameters are rejected. A vanished target or failed
server action returns a JSON-RPC error for the UI to explain and refresh.

## Verification

Normal tests use fake drivers, JSON fixtures, and QML controls without touching
host audio. They cover parsing, channel levels, absent metadata, stale targets,
permissions, invalid parameters, demand, device changes, service recovery,
keyboard actions, and stable delegates. `make e2e-audio` starts an isolated
PipeWire core, pipewire-pulse, and WirePlumber's policy-only profile (no hardware
monitors), creates virtual outputs and a silent stream, then exercises the real
backend/UI and service restart. It does not switch the user's real outputs or
restart the user's audio services. Physical Bluetooth transitions are not yet
verified on hardware.
