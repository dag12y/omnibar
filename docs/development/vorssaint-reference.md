# Vorssaint reference review

Reviewed on 2026-11-12. This is an initial targeted review, not a full audit.

- Upstream: https://github.com/vorssaint/vorssaint-utils
- Commit: `ce1276470d2b1115c8c3398f34e82079905fffba`
- Local checkout: `/home/dagm/Projects/vorssaint-reference` (separate from Omnibar).
- Source license declared upstream: GPL-3.0-or-later.
- Upstream `TRADEMARKS.md` reserves branding and trade dress separately.
- Omnibar currently uses MIT. This review introduces no upstream source or assets
  into Omnibar and makes no license changes. Direct code adaptation requires a
  licensing review; a language rewrite alone does not resolve that question.

## UI findings

The README's `docs/assets/readme/panel-system.png` shows a section navigator,
metric groups, compact historical charts, expandable rows, and persistent
settings/quit actions. These are useful interaction references, while Omnibar
should keep its own identity and follow the active Omarchy theme.

Relevant source locations (relative to upstream):

| Source | Finding | Omnibar direction |
| --- | --- | --- |
| `Sources/Vorssaint/UI/MenuPanel/MenuPanelView.swift` | Section navigation, metric focus, screen-height cap, visibility-specific monitoring needs | Separate overview from feature pages; cap the whole panel to screen height, with internal scrolling |
| `Sources/Vorssaint/UI/MenuPanel/PanelLayout.swift` | Stable section IDs, saved order/visibility, feature availability gates | Add navigation only for working modules; preserve layout preferences as modules evolve |
| `Sources/Vorssaint/UI/MenuPanel/MetricDetailView.swift` | Metric-specific detail and resource requirements | Make summary cards lead to dedicated details rather than growing the overview indefinitely |
| `Sources/Vorssaint/UI/Theme.swift` | Shared surface roles and contrast-aware styling | Extend our existing theme roles for readable light/dark and hover/focus states |
| `Sources/Vorssaint/Core/FeatureCatalog.swift` | Feature metadata and permissions distinct from runtime behavior | Extend module metadata for user-facing labels, icons, configuration, and availability |
| `Sources/Vorssaint/App/FeatureRuntime.swift` | Lazy service startup and immediate teardown when disabled | Keep optional background work tied to active modules and consumers |
| `Sources/Vorssaint/Services/Metrics/MonitorSamplingPolicy.swift` | Different sampling cadence by metric and foreground demand | Introduce explicit UI demand; hiding Omnibar currently leaves its subscriptions active |

## Features: useful behavior references

- Keep awake: timed sessions, manual stop, low-battery handling, cleanup on quit.
  Upstream `Services/KeepAwakeManager.swift` uses IOKit assertions and macOS
  power controls; the Linux implementation needs an appropriate inhibitor
  integration and explicit idle/display behavior.
- Audio mixer: per-app levels, output routing, and mute controls. Upstream
  `Services/Audio/AppVolumeMixer.swift` uses CoreAudio taps; Linux needs its
  own PipeWire integration. The UX can inform our design, not the platform code.
- Clipboard: review `Services/Clipboard/ClipboardHistoryService.swift` and
  `UI/MenuPanel/PanelClipboardView.swift` before designing retention, previews,
  favorites, and privacy controls. Those files have been located, not yet audited.
- Window controls: use upstream interaction behavior as a reference, with a
  Hyprland-specific adapter for implementation. Other compositors need separate
  capability detection rather than assuming Hyprland APIs exist everywhere.

## Recommended next increments

1. Finish panel interaction: screen-bound scrolling, keyboard navigation,
   Escape dismissal, accessible labels, and reliable theme-change updates.
2. Add visibility-aware monitor demand and reset rate baselines after idle.
3. Introduce feature navigation when the second working module is available;
   avoid placeholder controls for unimplemented features.
4. Build keep awake as a small complete interactive module, then per-app audio.
5. Review upstream feature-specific behavior before each module, and implement
   against Linux APIs independently unless code reuse is explicitly chosen.

The macOS app cannot be validated or run on this Linux host. This checkout is
for source and screenshot inspection; no upstream build scripts were executed.


## Audio follow-up (2026-11-12)

Reviewed `UI/MenuPanel/MixerSection.swift`, including app identity, percentage,
slider, mute, reset, and output controls. Omnibar now independently implements
playback-stream volume/mute and default output selection using PipeWire's Pulse
compatibility layer, with a separate Audio page. Boost, grouping, reset controls,
and per-app output routing remain outside this increment. See
[ADR-0012](../architecture/ADR-0012-audio-mixer.md).
