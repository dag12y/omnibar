# ADR-0002: UI Framework — Quickshell vs Raw Qt6 Quick/QML

Status: Accepted

## Context

Omnibar needs a desktop interface for Wayland/Hyprland: control center, floating panels, widgets, notifications. The original architecture proposal suggested Quickshell/QML without validating the choice. We must decide between Quickshell (a desktop-shell toolkit built on QtQuick) and raw Qt6 Quick/QML.

## Decision

Use **Quickshell + QML** for the desktop interface. Raw Qt6 Quick/QML is the documented fallback if Quickshell proves unsuitable.

## Alternatives considered

### Quickshell

A flexible toolkit for building desktop shells with QtQuick, for Wayland and X11.

**Advantages:**
- Wayland layer-shell, output hotplug, idle inhibition built-in
- QML types for bars, widgets, status notifier, system tray, audio mixer
- Already installed on this Omarchy system (`quickshell 0.3.2.r7.g2b4501a-1`)
- Is a dependency of the `omarchy` package itself
- Purpose-built for desktop utility shells — matches Omnibar's vision exactly

**Disadvantages:**
- Smaller community than raw Qt
- LGPL-3.0-only (stronger copyleft than Qt's LGPLv3/GPLv2 dual licensing)
- Mirror repository; upstream is at git.outfoxxed.me
- API surface is QML-only; no stable C++ API guarantee

### Raw Qt6 Quick/QML

The standard Qt Quick module: QML engine, visual canvas, input, animations, models/views, with a C++ API.

**Advantages:**
- Industry standard, massive documentation and ecosystem
- LGPLv3/GPLv2, well-understood licensing
- Stable C++ and QML APIs
- qmake6 and CMake both supported
- All Qt6 packages already installed on this system (qt6-declarative 6.12.0-2)

**Disadvantages:**
- No built-in layer-shell or desktop shell widgets
- Must build layer-shell, output management, idle inhibition from scratch
- More code to write for desktop-shell basics

## Consequences

**Positive:**
- Faster development: desktop-shell primitives (layer-shell, widgets, system tray) come from Quickshell
- Consistent with existing Omarchy Qt apps (`hype`, `monologue`, `omacut`)
- QML UI can connect to the Go backend via `Quickshell.Io.Socket`

**Negative:**
- LGPL-3.0-only license requires compliance (source availability for modifications)
- Smaller community means fewer third-party examples and slower issue resolution

## Trade-offs

We accept Quickshell's smaller community and LGPL-3.0 license in exchange for purpose-built desktop-shell primitives that would otherwise require significant custom C++/QML work. Raw Qt6 Quick remains the fallback if Quickshell's API stability or licensing becomes a blocker.

## Quickshell compatibility

- **Upstream license:** Quickshell is dual-licensed under LGPL-3.0-only and GPL-3.0-only (as shown in the repository's LICENSE and LICENSE-GPL files).
- **API compatibility:** Quickshell's QML API may introduce breaking changes between versions. QML API compatibility must be tested against the pinned Quickshell version before each upgrade.
- **Version pinning:** Pin and test against a known Quickshell version (currently `0.3.2.r7.g2b4501a-1` on this Omarchy system). Do not track `quickshell-git` or untracked upstream commits without regression testing.
- **Source availability:** Because Quickshell is LGPL-3.0-only, any modifications to Quickshell itself must be made available in source form when distributing the combined work.

## Trade-offs

We accept Quickshell's smaller community and LGPL-3.0 license in exchange for purpose-built desktop-shell primitives that would otherwise require significant custom C++/QML work. Raw Qt6 Quick remains the fallback if Quickshell's API stability or licensing becomes a blocker.

## References

- Quickshell repository: https://github.com/quickshell-mirror/quickshell (website: https://quickshell.outfoxxed.me)
- Local package: `quickshell 0.3.2.r7.g2b4501a-1` (pacman)
- Qt Quick documentation: https://doc.qt.io/qt-6/qtquick-index.html
- Existing Omarchy Qt apps: `hype 0.6.0`, `monologue 0.3.1`, `omacut 0.7.2`
