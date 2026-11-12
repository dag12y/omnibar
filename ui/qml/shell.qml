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
    ThemeLoader {}
    property bool opened: true
    property string expanded: ""
    property var cpuHistory: []
    property var memoryHistory: []
    readonly property var snap: client.snapshots["system_monitor"] || ({})
    readonly property string state: client.snapshotStates["system_monitor"] || "pending"
    readonly property bool stale: state === "stale" || !client.backendConnected
    function field(group, key) { return snap[group] ? snap[group][key] : undefined }
    function percent(group) {
        const total = field(group, "total")
        return total > 0 ? field(group, "used") / total * 100 : 0
    }
    function bytes(value) { return typeof value === "number" ? Format.formatBytes(value) : "—" }
    function rate(value) { return typeof value === "number" ? Format.formatBytes(value) + "/s" : "—" }
    BackendClient {
        id: client
        onModuleEvent: (module, event, data) => {
            if (module !== "system_monitor") return
            if (event === "cpu" && typeof data.percent === "number")
                root.cpuHistory = root.cpuHistory.concat([data.percent]).slice(-40)
            if (event === "memory" && data.total > 0)
                root.memoryHistory = root.memoryHistory.concat([data.used / data.total * 100]).slice(-40)
        }
        onHandshakeSucceeded: { root.cpuHistory = []; root.memoryHistory = [] }
    }
    IpcHandler {
        target: "panel"
        function toggle(): void { root.opened = !root.opened }
        function show(): void { root.opened = true }
        function hide(): void { root.opened = false }
    }
    PanelWindow {
        id: panel
        visible: root.opened
        anchors { top: true; right: true }
        margins { top: 16; right: 16 }
        implicitWidth: Math.min(408, screen.width - 32)
        implicitHeight: Math.min(body.implicitHeight + 40, screen.height - 64)
        WlrLayershell.namespace: "omnibar-panel"
        WlrLayershell.keyboardFocus: root.opened ? WlrKeyboardFocus.OnDemand : WlrKeyboardFocus.None
        color: "transparent"
        exclusionMode: ExclusionMode.Ignore
        mask: Region { item: surface }
        BackgroundEffect.blurRegion: Region { item: surface }
        Rectangle {
            id: surface
            anchors.fill: parent
            radius: 24
            color: Qt.alpha(Theme.background, 0.96)
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
                    spacing: 10
                    Rectangle {
                        width: 32; height: 32; radius: 10
                        color: Qt.alpha(Theme.accent, 0.15)
                        Text { anchors.centerIn: parent; text: "◈"; color: Theme.accent; font.pixelSize: 22 }
                    }
                    ColumnLayout {
                        spacing: 3
                        Layout.fillWidth: true
                        Text { text: "Omnibar"; color: Theme.foreground; font.pixelSize: 17; font.weight: Font.DemiBold }
                        Text { text: "YOUR SYSTEM, AT A GLANCE"; color: Theme.muted; font.pixelSize: 9; font.letterSpacing: 1 }
                    }
                    PanelButton {
                        text: "×"
                        description: "Close panel (Escape)"
                        onClicked: root.opened = false
                    }
                }
                Rectangle {
                    visible: root.state === "denied" || root.state === "unavailable" || root.state === "failed" || root.state === "pending" || !client.backendConnected
                    Layout.fillWidth: true
                    implicitHeight: notice.implicitHeight + 24
                    radius: 12; color: Qt.alpha(Theme.accent, 0.08)
                    Text {
                        id: notice
                        anchors.fill: parent; anchors.margins: 12
                        wrapMode: Text.WordWrap; color: Theme.muted; font.pixelSize: 12
                        text: root.state === "denied" ? "Monitoring needs the system.info permission in your Omnibar config."
                            : root.state === "unavailable" ? "System monitoring is disabled."
                            : root.state === "failed" ? "Could not load system metrics."
                            : root.stale && root.snap.sampled_at ? "Connection lost · showing the last reading. Reconnecting…"
                            : client.backendConnected ? "Loading system metrics…" : "Connecting to your system…"
                    }
                }
                RowLayout {
                    spacing: 12
                    Layout.fillWidth: true
                    SummaryCard {
                        Layout.fillWidth: true
                        label: "Processor"; iconName: "cpu"; stale: root.stale
                        value: typeof root.field("cpu", "percent") === "number" ? Math.round(root.field("cpu", "percent")) + "%" : "—"
                        detail: (root.field("cpu", "cores") || []).length + " logical cores"
                        samples: root.cpuHistory
                        onActivated: root.expanded = root.expanded === "cpu" ? "" : "cpu"
                    }
                    SummaryCard {
                        Layout.fillWidth: true
                        label: "Memory"; iconName: "memory"; stale: root.stale
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
                                    label: "Core " + (index + 1); iconName: "cpu"
                                    value: Format.formatPercent(modelData); percent: modelData; stale: root.stale
                                }
                            }
                        }
                    }
                    MetricBar {
                        visible: root.expanded === "memory"
                        label: "Swap"; iconName: "swap"; stale: root.stale
                        value: root.bytes(root.field("swap", "used")) + " / " + root.bytes(root.field("swap", "total"))
                        percent: root.percent("swap")
                    }
                }
                Rectangle { Layout.fillWidth: true; height: 1; color: Theme.border }
                MetricBar {
                    label: "Network"; iconName: "network"; showTrack: false; stale: root.stale
                    value: "↓ " + root.rate(root.field("network", "rx_bytes_sec"))
                    detail: "↑ " + root.rate(root.field("network", "tx_bytes_sec"))
                }
                MetricBar {
                    label: "Storage"; iconName: "disk"; stale: root.stale
                    value: root.bytes(root.field("disk", "used")) + " / " + root.bytes(root.field("disk", "total"))
                    detail: "Read " + root.rate(root.field("disk", "read_bytes_sec")) + "   ·   Write " + root.rate(root.field("disk", "write_bytes_sec"))
                    percent: root.percent("disk")
                }
                MetricBar {
                    visible: root.field("battery", "present") === true
                    label: "Battery"; iconName: "battery"; stale: root.stale
                    value: Format.formatPercent(root.field("battery", "percent") || 0)
                    detail: (root.field("battery", "state") || "Unknown") + (root.field("battery", "power_watts") > 0 ? " · " + root.field("battery", "power_watts").toFixed(1) + " W" : "")
                    percent: root.field("battery", "percent") || 0
                }
                RowLayout {
                    Layout.fillWidth: true
                    Rectangle { width: 5; height: 5; radius: 3; color: root.stale ? Theme.muted : Theme.accent }
                    Text { text: root.stale ? "Offline" : root.state === "fresh" ? "Live monitoring" : "Waiting for metrics"; color: Theme.muted; font.pixelSize: 10; Layout.fillWidth: true }
                    PanelButton {
                        text: client.backendConnected ? "Refresh ↻" : "Retry ↻"
                        description: client.backendConnected ? "Refresh system metrics" : "Reconnect to backend"
                        onClicked: client.backendConnected ? client.refreshHandshake() : client.retryNow()
                    }
                }
                    }
                }
            }
        }
    }
}
