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
            if (name === "system" || name === "audio" || name === "clipboard")
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
                                    text: root.page === "audio" ? "YOUR SOUND, YOUR CONTROL" : root.page === "clipboard" ? "COPIED, KEPT WITHIN REACH" : "YOUR SYSTEM, AT A GLANCE"
                                    color: Theme.muted
                                    font.pixelSize: 9
                                    font.letterSpacing: 1
                                }
                            }
                            Item {
                                Layout.fillWidth: true
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
                                text: "Clipboard"
                                selected: root.page === "clipboard"
                                description: "Show clipboard history"
                                onClicked: root.page = "clipboard"
                            }
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
                                text: !client.backendConnected ? "Offline" : root.page === "audio" ? (audio.state === "ready" ? "Audio connected" : "Audio " + audio.state) : root.page === "clipboard" ? "Session clipboard history" : root.state === "fresh" ? "Live monitoring" : "Waiting for metrics"
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
