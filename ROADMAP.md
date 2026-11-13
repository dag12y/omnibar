# Development Roadmap

## Phase 0 - Planning
- [ ] Research Vorssaint features
- [ ] Audit Linux feature compatibility
- [ ] Define functional requirements
- [ ] Design system architecture
- [ ] Choose technology stack

## Phase 1 - Core Foundation
- [x] Go backend starts
- [x] Quickshell UI launches
- [x] UI connects to Go through the Unix socket
- [x] JSON-RPC handshake and request/response work
- [x] Basic module registry works
- [x] TOML configuration loads and validates
- [x] Logging and graceful shutdown work
- [x] A minimal working UI demonstrates the connection

## Phase 2 - System Monitoring
System monitoring suite: event delivery, resource metrics, UI cards, and the v0.1.0 release.
- [x] Event subscription and delivery (M9)
- [x] CPU monitoring (M10, M11)
- [x] RAM and swap monitoring (M10, M11)
- [x] Disk monitoring: filesystem storage and I/O throughput (M12)
- [x] Network monitoring: interface throughput and transfer rates (M13)
- [x] Battery and power monitoring: charge levels, status, and power draw (M14)
- [x] Release v0.1.0 (M15)
- [ ] GPU monitoring (future Phase 2 milestone)
- [ ] System alerts and history (requires storage / SQLite)

## Phase 3 - Window Management
- [x] Installed application launcher: search and explicit desktop-entry launch (M25)
- [x] Hyprland running-window switcher: search, exact focus, and workspace switching (M24)
- [x] Optional in-memory window previews in Windows and Search (M27)
- [ ] Window layouts

## Phase 4 - Clipboard and Files
- [x] Session-only text clipboard history, search, copy, delete, and clear (M20)
- [x] Explicitly saved text favorites, restart restore, filter, and removal (M21)
- [x] Session File shelf: local references, search, folder opening, and path copy (M22)
- [x] Clipboard utilities: trim, case conversion, and remove-line-breaks copy (M23)

## Phase 5 - Audio and Input
- [x] Per-application playback-stream volume and mute
- [x] Default playback output selection
- [x] Audio page, live device/stream updates, and service reconnection
- [x] Per-app playback-stream output routing
- [x] Default microphone selection, gain, and mute
- [x] Recording-app/source metadata
- [ ] Per-recording-app input routing and live sound-level meters
- [ ] Mouse customization
- [ ] Keyboard customization

## Phase 6 - Productivity
- [x] Quick panel: keyboard navigation, screen-aware scrolling, and System/Audio/Windows/Clipboard/Files pages
- [x] App/window command bar: unified search and explicit Switch/Launch (M26)
- [ ] Scratchpad
- [x] Keep awake: timed logind/Wayland inhibition with countdown and Stop
- [ ] Color picker
- [ ] OCR and QR reader

## Phase 7 - Capture and Media
- [ ] Screenshots
- [ ] Screen recording
- [ ] GIF creation
- [ ] Media conversion
- [ ] Media compression

## Phase 8 - System Management
- [ ] Application manager
- [ ] Package management
- [ ] Process manager
- [ ] Port manager
- [ ] Display controls
- [ ] Bluetooth controls

## Phase 9 - Dynamic Island
- [ ] Floating interface
- [ ] Music controls
- [ ] Notifications
- [ ] Calendar
- [ ] Timers
- [ ] Download progress
- [ ] AI agent monitoring

## Phase 10 - Developer Tools
- [ ] Git dashboard
- [ ] Docker manager
- [ ] SSH manager
- [ ] Service manager
- [ ] Floating terminal
- [ ] Workspace manager

## Phase 11 - Distribution
- [ ] Package and release integration
- [ ] Module ecosystem
- [ ] GitHub releases
- [ ] Installation documentation
- [ ] Stable release
