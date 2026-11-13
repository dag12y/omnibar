import QtQuick
import QtQuick.Layouts
import QtQuick.Controls
import Quickshell
import Quickshell.Io
import Quickshell.Wayland
import "components"
import "components/format.js" as Format

ShellRoot {
    id: root
    ThemeLoader {
        active: root.opened
    }
    function revealItem(item) {
        if (!root.opened || !item) return;
        const flick = scroll.contentItem;
        const top = item.mapToItem(flick, 0, 0).y;
        let offset = 0;
        if (top < 0) offset = top;
        else if (top + item.height > flick.height) offset = top + item.height - flick.height;
        flick.contentY = Math.max(0, Math.min(flick.contentHeight - flick.height, flick.contentY + offset));
    }
    function openSearch() {
        root.page = "search";
        root.opened = true;
        Qt.callLater(unifiedSearchPage.focusQuery);
    }
    property string page: "system"
    onPageChanged: {
        cpuHistory = [];
        memoryHistory = [];
    }
    property bool opened: true
    onOpenedChanged: {
        cpuHistory = [];
        memoryHistory = [];
    }
    property string expanded: ""
    property var cpuHistory: []
    property var memoryHistory: []
    readonly property var snap: client.snapshots["system_monitor"] || ({})
    readonly property string state: client.snapshotStates["system_monitor"] || "pending"
    readonly property bool stale: state === "stale" || !client.backendConnected
    function field(group, key) {
        return snap[group] ? snap[group][key] : undefined;
    }
    function percent(group) {
        const total = field(group, "total");
        return total > 0 ? field(group, "used") / total * 100 : 0;
    }
    function bytes(value) {
        return typeof value === "number" ? Format.formatBytes(value) : "—";
    }
    function rate(value) {
        return typeof value === "number" ? Format.formatBytes(value) + "/s" : "—";
    }
    BackendClient {
        id: client
        monitoringActive: root.opened && root.page === "system"
        onModuleEvent: (module, event, data) => {
            if (module !== "system_monitor")
                return;
            if (event === "cpu" && typeof data.percent === "number")
                root.cpuHistory = root.cpuHistory.concat([data.percent]).slice(-40);
            if (event === "memory" && data.total > 0)
                root.memoryHistory = root.memoryHistory.concat([data.used / data.total * 100]).slice(-40);
        }
        onHandshakeSucceeded: {
            root.cpuHistory = [];
            root.memoryHistory = [];
        }
    }
    AudioController {
        id: audio
        client: client
        active: root.opened && root.page === "audio"
    }
    ClipboardController {
        id: history
        client: client
        active: root.opened && root.page === "clipboard"
    }
    FileShelfController {
        id: shelf
        client: client
        active: root.opened && root.page === "files"
    }
    NotesController {
        id: notes
        client: client
        active: root.opened && root.page === "notes"
    }
    AppController {
        id: apps
        client: client
        active: root.opened && (root.page === "apps" || root.page === "search")
        onAppLaunched: root.opened = false
    }
    WindowController {
        id: windows
        client: client
        active: root.opened && (root.page === "windows" || root.page === "search")
        onWindowFocused: root.opened = false
    }
    PreviewController {
        id: previews
        client: client
        active: root.opened && (root.page === "windows" || root.page === "search")
        scope: root.page
        allowed: windows.state === "ready" && windows.snapshot.previewsAllowed === true
        eligibleAddresses: (windows.snapshot.windows || []).map(window => window.address)
    }
    // A separate mapped surface keeps display-idle inhibition alive when the
    // dashboard is hidden. It takes no keyboard focus or pointer input.
    PanelWindow {
        id: awakeSurface
        visible: client.backendConnected && client.keepAwakeStatus.active === true
        anchors {
            bottom: true
            right: true
        }
        implicitWidth: 1
        implicitHeight: 1
        color: "transparent"
        exclusionMode: ExclusionMode.Ignore
        WlrLayershell.namespace: "omnibar-keep-awake"
        WlrLayershell.keyboardFocus: WlrKeyboardFocus.None
        mask: Region {}
        Rectangle {
            anchors.fill: parent
            color: Theme.accent
            opacity: 0.01
        }
        IdleInhibitor {
            window: awakeSurface
            enabled: awakeSurface.visible
        }
    }
    IpcHandler {
        target: "panel"
        function showSearch(): void {
            root.openSearch();
        }
        function showNotes(): void {
            root.page = "notes";
            root.opened = true;
        }
        function showApps(): void {
            root.page = "apps";
            root.opened = true;
        }
        function showWindows(): void {
            root.page = "windows";
            root.opened = true;
        }
        function showFiles(): void {
            root.page = "files";
            root.opened = true;
        }
        function showClipboard(): void {
            root.page = "clipboard";
            root.opened = true;
        }
        function showMicrophone(): void {
            root.page = "audio";
            root.opened = true;
            audioPage.section = "microphone";
        }
        function toggle(): void {
            root.opened = !root.opened;
        }
        function show(): void {
            root.opened = true;
        }
        function selectPage(name: string): void {
            if (name === "system" || name === "audio" || name === "clipboard" || name === "files" || name === "windows" || name === "apps" || name === "search" || name === "notes")
                root.page = name;
        }
        function hide(): void {
            root.opened = false;
        }
    }
    PanelWindow {
        id: panel
        visible: root.opened
        anchors {
            top: true
            right: true
        }
        margins {
            top: 16
            right: 16
        }
        implicitWidth: Math.min(408, screen.width - 32)
        implicitHeight: Math.min(body.implicitHeight + 40, screen.height - 64)
        WlrLayershell.namespace: "omnibar-panel"
        WlrLayershell.keyboardFocus: root.opened ? WlrKeyboardFocus.OnDemand : WlrKeyboardFocus.None
        color: "transparent"
        exclusionMode: ExclusionMode.Ignore
        mask: Region {
            item: surface
        }
        BackgroundEffect.blurRegion: Region {
            item: surface
        }
        Rectangle {
            id: surface
            anchors.fill: parent
            radius: 24
            color: Theme.background
            border.color: Theme.border
            FocusScope {
                anchors.fill: parent
                focus: true
                Keys.onEscapePressed: root.opened = false
                Shortcut {
                    sequence: "Ctrl+K"
                    enabled: root.opened
                    onActivated: root.openSearch()
                }
                ScrollView {
                    id: scroll
                    anchors.fill: parent
                    anchors.margins: 20
                    contentWidth: availableWidth
                    clip: true
                    ScrollBar.horizontal.policy: ScrollBar.AlwaysOff
                    ColumnLayout {
                        id: body
                        width: scroll.availableWidth
                        spacing: 18
                        RowLayout {
                            Layout.fillWidth: true
                            spacing: 10
                            Rectangle {
                                width: 32
                                height: 32
                                radius: 10
                                color: Qt.alpha(Theme.accent, 0.15)
                                Text {
                                    anchors.centerIn: parent
                                    text: "◈"
                                    color: Theme.accent
                                    font.pixelSize: 22
                                }
                            }
                            ColumnLayout {
                                spacing: 3
                                Layout.fillWidth: true
                                Text {
                                    text: "Omnibar"
                                    color: Theme.foreground
                                    font.pixelSize: 17
                                    font.weight: Font.DemiBold
                                }
                                Text {
                                    text: root.page === "search" ? "APPS AND WINDOWS, TOGETHER" : root.page === "apps" ? "YOUR APPS, WITHIN REACH" : root.page === "audio" ? "YOUR SOUND, YOUR CONTROL" : root.page === "windows" ? "PICK UP WHERE YOU LEFT OFF" : root.page === "files" ? "FILES WITHIN REACH" : root.page === "clipboard" ? "COPIED, KEPT WITHIN REACH" : "YOUR SYSTEM, AT A GLANCE"
                                    color: Theme.muted
                                    font.pixelSize: 9
                                    font.letterSpacing: 1
                                }
                            }
                            Item {
                                Layout.fillWidth: true
                            }
                            PanelButton {
                                text: "Search"
                                selected: root.page === "search"
                                description: "Search apps and windows (Ctrl+K)"
                                onClicked: root.openSearch()
                            }
                            PanelButton {
                                text: "×"
                                description: "Close panel (Escape)"
                                onClicked: root.opened = false
                            }
                        }
                        RowLayout {
                            Layout.fillWidth: true
                            spacing: 8
                            PanelButton {
                                Layout.fillWidth: true
                                text: "System"
                                selected: root.page === "system"
                                description: "Show system monitoring"
                                onClicked: root.page = "system"
                            }
                            PanelButton {
                                Layout.fillWidth: true
                                text: "Audio"
                                selected: root.page === "audio"
                                description: "Show audio mixer"
                                onClicked: root.page = "audio"
                            }
                            PanelButton {
                                Layout.fillWidth: true
                                text: "Windows"
                                selected: root.page === "windows"
                                description: "Switch between running windows"
                                onClicked: root.page = "windows"
                            }
                        }
                        RowLayout {
                            Layout.fillWidth: true
                            spacing: 8
                            PanelButton {
                                Layout.fillWidth: true
                                text: "Clipboard"
                                selected: root.page === "clipboard"
                                description: "Show clipboard history"
                                onClicked: root.page = "clipboard"
                            }
                            PanelButton {
                                Layout.fillWidth: true
                                text: "Files"
                                selected: root.page === "files"
                                description: "Show file shelf"
                                onClicked: root.page = "files"
                            }
                            PanelButton {
                                Layout.fillWidth: true
                                text: "Apps"
                                selected: root.page === "apps"
                                description: "Launch installed applications"
                                onClicked: root.page = "apps"
                            }
                        }
                        RowLayout {
                            Layout.fillWidth: true
                            PanelButton {
                                text: "Notes"
                                selected: root.page === "notes"
                                description: "Open Scratchpad"
                                onClicked: root.page = "notes"
                            }
                        }
                        NotesPage {
                            visible: root.page === "notes"
                            Layout.fillWidth: true
                            controller: notes
                            onRevealRequested: item => root.revealItem(item)
                        }
                        SearchPage {
                            id: unifiedSearchPage
                            visible: root.page === "search"
                            active: root.opened && root.page === "search"
                            Layout.fillWidth: true
                            windowSnapshot: active ? windows.snapshot : ({ windows: [] })
                            appSnapshot: active ? apps.snapshot : ({ apps: [] })
                            windowState: windows.state
                            appState: apps.state
                            windowError: windows.errorText
                            appError: apps.errorText
                            busy: windows.busy || apps.busy
                            onFocusRequested: address => windows.request("windows.focus", { address: address })
                            onLaunchRequested: appID => apps.request("apps.launch", { id: appID })
                            onRefreshRequested: {
                                windows.request("windows.getSnapshot");
                                apps.request("apps.getSnapshot");
                            }
                            preview: previews
                            onHighlightRequested: item => root.revealItem(item)
                        }
                        AppPage {
                            visible: root.page === "apps"
                            Layout.fillWidth: true
                            snapshot: root.page === "apps" ? apps.snapshot : ({ apps: [] })
                            state: apps.state
                            errorText: apps.errorText
                            busy: apps.busy
                            onLaunchRequested: appID => apps.request("apps.launch", { id: appID })
                            onRefreshRequested: apps.request("apps.getSnapshot")
                        }
                        WindowPage {
                            visible: root.page === "windows"
                            active: root.opened && root.page === "windows"
                            Layout.fillWidth: true
                            snapshot: root.page === "windows" ? windows.snapshot : ({ windows: [] })
                            preview: previews
                            onRevealRequested: item => root.revealItem(item)
                            onControlRequested: (method, params) => windows.request(method, params)
                            state: windows.state
                            errorText: windows.errorText
                            busy: windows.busy
                            onFocusRequested: address => windows.request("windows.focus", {
                                    address: address
                                })
                            onRefreshRequested: windows.request("windows.getSnapshot")
                        }
                        FileShelfPage {
                            visible: root.page === "files"
                            Layout.fillWidth: true
                            snapshot: shelf.snapshot
                            state: shelf.state
                            busy: shelf.busy
                            errorText: shelf.errorText
                            onPathsRequested: paths => shelf.request("shelf.add", {
                                    paths: paths
                                })
                            onRemoveRequested: id => shelf.request("shelf.remove", {
                                    id: id
                                })
                            onCopyRequested: id => shelf.request("shelf.copyPath", {
                                    id: id
                                })
                            onFolderRequested: id => shelf.request("shelf.openFolder", {
                                    id: id
                                })
                            onClearRequested: shelf.request("shelf.clear")
                            onRefreshRequested: shelf.request("shelf.getSnapshot")
                        }
                        ClipboardPage {
                            visible: root.page === "clipboard"
                            Layout.fillWidth: true
                            snapshot: history.snapshot
                            state: history.state
                            busy: history.busy
                            errorText: history.errorText
                            onCollectionRequested: collecting => history.request("clipboard.setCollecting", {
                                    collecting: collecting
                                })
                            onFavoriteRequested: (id, favorite) => history.request("clipboard.setFavorite", {
                                    id: id,
                                    favorite: favorite
                                })
                            onTransformRequested: (id, operation) => history.request("clipboard.transformCopy", {
                                    id: id,
                                    operation: operation
                                })
                            onCopyRequested: id => history.request("clipboard.copy", {
                                    id: id
                                })
                            onDeleteRequested: id => history.request("clipboard.delete", {
                                    id: id
                                })
                            onClearRequested: history.request("clipboard.clear")
                            onRefreshRequested: history.request("clipboard.getSnapshot")
                        }
                        AudioPage {
                            id: audioPage
                            visible: root.page === "audio"
                            Layout.fillWidth: true
                            snapshot: audio.snapshot
                            state: audio.state
                            errorText: audio.errorText
                            busy: audio.busy
                            resolveIcon: name => name ? Quickshell.iconPath(name, true) : ""
                            onVolumeRequested: (key, volume) => audio.request("audio.setVolume", {
                                    key: key,
                                    volume: volume
                                })
                            onMuteRequested: (key, muted) => audio.request("audio.setMute", {
                                    key: key,
                                    muted: muted
                                })
                            onRouteRequested: (key, output) => audio.request("audio.moveStream", {
                                    key: key,
                                    output: output
                                })
                            onOutputRequested: output => audio.request("audio.setDefaultOutput", {
                                    output: output
                                })
                            onInputVolumeRequested: (input, volume) => audio.request("audio.setInputVolume", {
                                    input: input,
                                    volume: volume
                                })
                            onInputMuteRequested: (input, muted) => audio.request("audio.setInputMute", {
                                    input: input,
                                    muted: muted
                                })
                            onInputRequested: input => audio.request("audio.setDefaultInput", {
                                    input: input
                                })
                            onRefreshRequested: audio.request("audio.getSnapshot")
                        }
                        ColumnLayout {
                            visible: root.page === "system"
                            Layout.fillWidth: true
                            spacing: 18
                            Rectangle {
                                visible: root.state === "denied" || root.state === "unavailable" || root.state === "failed" || root.state === "pending" || !client.backendConnected
                                Layout.fillWidth: true
                                implicitHeight: notice.implicitHeight + 24
                                radius: 12
                                color: Qt.alpha(Theme.accent, 0.08)
                                Text {
                                    id: notice
                                    anchors.fill: parent
                                    anchors.margins: 12
                                    wrapMode: Text.WordWrap
                                    color: Theme.muted
                                    font.pixelSize: 12
                                    text: root.state === "denied" ? "Monitoring needs the system.info permission in your Omnibar config." : root.state === "unavailable" ? "System monitoring is disabled." : root.state === "failed" ? "Could not load system metrics." : root.stale && root.snap.sampled_at ? "Connection lost · showing the last reading. Reconnecting…" : client.backendConnected ? "Loading system metrics…" : "Connecting to your system…"
                                }
                            }
                            RowLayout {
                                spacing: 12
                                Layout.fillWidth: true
                                SummaryCard {
                                    Layout.fillWidth: true
                                    label: "Processor"
                                    iconName: "cpu"
                                    stale: root.stale
                                    value: typeof root.field("cpu", "percent") === "number" ? Math.round(root.field("cpu", "percent")) + "%" : "—"
                                    detail: root.field("cpu", "core_count") > 0 ? root.field("cpu", "core_count") + " logical cores" : "Waiting for sample"
                                    samples: root.cpuHistory
                                    onActivated: root.expanded = root.expanded === "cpu" ? "" : "cpu"
                                }
                                SummaryCard {
                                    Layout.fillWidth: true
                                    label: "Memory"
                                    iconName: "memory"
                                    stale: root.stale
                                    value: root.field("memory", "total") > 0 ? Math.round(root.percent("memory")) + "%" : "—"
                                    detail: root.bytes(root.field("memory", "used")) + " / " + root.bytes(root.field("memory", "total"))
                                    samples: root.memoryHistory
                                    onActivated: root.expanded = root.expanded === "memory" ? "" : "memory"
                                }
                            }
                            ColumnLayout {
                                visible: root.expanded.length > 0
                                Layout.fillWidth: true
                                spacing: 8
                                Flickable {
                                    visible: root.expanded === "cpu"
                                    Layout.fillWidth: true
                                    implicitHeight: Math.min(coreRows.implicitHeight, 220)
                                    contentHeight: coreRows.implicitHeight
                                    clip: true
                                    boundsBehavior: Flickable.StopAtBounds
                                    ColumnLayout {
                                        id: coreRows
                                        width: parent.width
                                        spacing: 8
                                        Repeater {
                                            model: root.expanded === "cpu" ? root.field("cpu", "cores") || [] : []
                                            MetricBar {
                                                required property int index
                                                required property real modelData
                                                label: "Core " + (index + 1)
                                                iconName: "cpu"
                                                value: Format.formatPercent(modelData)
                                                percent: modelData
                                                stale: root.stale
                                            }
                                        }
                                    }
                                }
                                MetricBar {
                                    visible: root.expanded === "memory"
                                    label: "Swap"
                                    iconName: "swap"
                                    stale: root.stale
                                    value: root.bytes(root.field("swap", "used")) + " / " + root.bytes(root.field("swap", "total"))
                                    percent: root.percent("swap")
                                }
                            }
                            KeepAwakeCard {
                                Layout.fillWidth: true
                                active: client.keepAwakeStatus.active === true
                                expiresAt: client.keepAwakeStatus.expires_at || ""
                                connected: client.backendConnected
                                busy: client.keepAwakeBusy
                                available: client.subscriptions["keep_awake"] === "subscribed"
                                denied: client.subscriptions["keep_awake"] === "denied"
                                errorText: client.keepAwakeError
                                clockRunning: root.opened && root.page === "system"
                                onStartRequested: seconds => client.keepAwakeRequest("keepAwake.start", seconds)
                                onStopRequested: client.keepAwakeRequest("keepAwake.stop")
                            }
                            Rectangle {
                                Layout.fillWidth: true
                                height: 1
                                color: Theme.border
                            }
                            MetricBar {
                                label: "Network"
                                iconName: "network"
                                showTrack: false
                                stale: root.stale
                                value: "↓ " + root.rate(root.field("network", "rx_bytes_sec"))
                                detail: "↑ " + root.rate(root.field("network", "tx_bytes_sec"))
                            }
                            MetricBar {
                                label: "Storage"
                                iconName: "disk"
                                stale: root.stale
                                value: root.bytes(root.field("disk", "used")) + " / " + root.bytes(root.field("disk", "total"))
                                detail: "Read " + root.rate(root.field("disk", "read_bytes_sec")) + "   ·   Write " + root.rate(root.field("disk", "write_bytes_sec"))
                                percent: root.percent("disk")
                            }
                            MetricBar {
                                visible: root.field("battery", "present") === true
                                label: "Battery"
                                iconName: "battery"
                                stale: root.stale
                                value: Format.formatPercent(root.field("battery", "percent") || 0)
                                detail: (root.field("battery", "state") || "Unknown") + (root.field("battery", "power_watts") > 0 ? " · " + root.field("battery", "power_watts").toFixed(1) + " W" : "")
                                percent: root.field("battery", "percent") || 0
                            }
                        }
                        RowLayout {
                            Layout.fillWidth: true
                            Rectangle {
                                width: 5
                                height: 5
                                radius: 3
                                color: root.stale ? Theme.muted : Theme.accent
                            }
                            Text {
                                text: !client.backendConnected ? "Offline" : root.page === "audio" ? (audio.state === "ready" ? "Audio connected" : "Audio " + audio.state) : root.page === "search" ? "Apps and running windows" : root.page === "apps" ? "Installed applications" : root.page === "windows" ? "Running Hyprland windows" : root.page === "files" ? "Session file shelf" : root.page === "clipboard" ? "Clipboard · recent and saved" : root.state === "fresh" ? "Live monitoring" : "Waiting for metrics"
                                color: Theme.muted
                                font.pixelSize: 10
                                Layout.fillWidth: true
                            }
                            PanelButton {
                                text: client.backendConnected ? "Refresh ↻" : "Retry ↻"
                                description: client.backendConnected ? "Refresh system metrics" : "Reconnect to backend"
                                visible: root.page === "system" || !client.backendConnected
                                onClicked: client.backendConnected ? (root.page === "audio" ? audio.request("audio.getSnapshot") : client.refreshHandshake()) : client.retryNow()
                            }
                        }
                    }
                }
            }
        }
    }
}
