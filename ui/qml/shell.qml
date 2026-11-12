import QtQuick
import QtQuick.Layouts
import Quickshell
import Quickshell.Wayland

import "components"
import "components/format.js" as Format

ShellRoot {
    BackendClient {
        id: client
    }

    PanelWindow {
        id: panel

        anchors {
            top: true
            left: true
        }

        margins {
            top: 12
            left: 12
        }

        implicitWidth: surface.implicitWidth
        implicitHeight: surface.implicitHeight
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

            implicitWidth: 380
            implicitHeight: body.implicitHeight + 28
            width: implicitWidth
            height: implicitHeight
            radius: 22
            color: Qt.rgba(0.09, 0.09, 0.11, 0.86)
            border.width: 1
            border.color: Qt.rgba(1, 1, 1, 0.14)

            ColumnLayout {
                id: body
                x: 14
                y: 14
                width: parent.width - 28
                spacing: 12

                RowLayout {
                    Layout.fillWidth: true
                    spacing: 8

                    Text {
                        text: "Omnibar"
                        color: "#f4f6f8"
                        font.pixelSize: 15
                        font.weight: Font.DemiBold
                        Layout.fillWidth: true
                    }

                    ConnectionStatus {
                        status: client.status
                        label: false
                        Layout.alignment: Qt.AlignVCenter
                    }
                }

                Text {
                    Layout.fillWidth: true
                    text: {
                        if (client.backendConnected) {
                            const snap = client.snapshots["system_monitor"]
                            if (snap && snap.sampled_at)
                                return "Live · " + Format.formatSampledAt(snap.sampled_at)
                            return "Live"
                        }
                        if (client.connecting)
                            return "Connecting…"
                        if (client.lastError.length > 0)
                            return client.lastError
                        if (client.backendUnavailable && client.retryDelaySeconds > 0)
                            return "Retrying in " + client.retryDelaySeconds + "s"
                        return "Backend unavailable"
                    }
                    color: "#9aa3b2"
                    font.pixelSize: 11
                    elide: Text.ElideRight
                    wrapMode: Text.NoWrap
                }

                ModuleCard {
                    id: metricCard
                    Layout.fillWidth: true
                    Layout.preferredHeight: implicitHeight
                    stale: client.snapshotStates["system_monitor"] === "stale"

                    function field(path) {
                        const snap = client.snapshots["system_monitor"]
                        const parts = path.split(".")
                        let v = snap
                        for (const p of parts) {
                            if (!v)
                                return undefined
                            v = v[p]
                        }
                        return v
                    }

                    Text {
                        text: "SYSTEM"
                        color: "#8b93a3"
                        font.pixelSize: 10
                        font.weight: Font.DemiBold
                        font.letterSpacing: 0.8
                        Layout.fillWidth: true
                        Layout.topMargin: 2
                    }

                    MetricBar {
                        label: "CPU"
                        iconName: "cpu"
                        value: {
                            const cores = metricCard.field("cpu.cores")
                            const p = metricCard.field("cpu.percent")
                            if (typeof p !== "number")
                                return "\u2014"
                            const count = cores ? cores.length : 0
                            return Format.formatPercent(p) + (count > 0 ? "  ·  " + count + " cores" : "")
                        }
                        percent: {
                            const p = metricCard.field("cpu.percent")
                            return typeof p === "number" ? p : 0
                        }
                        stale: client.snapshotStates["system_monitor"] === "stale"
                    }

                    MetricBar {
                        label: "Memory"
                        iconName: "memory"
                        value: {
                            const u = metricCard.field("memory.used")
                            const t = metricCard.field("memory.total")
                            if (typeof u === "number" && typeof t === "number" && t > 0)
                                return Format.formatBytes(u) + " / " + Format.formatBytes(t)
                            return "\u2014"
                        }
                        percent: {
                            const u = metricCard.field("memory.used")
                            const t = metricCard.field("memory.total")
                            if (typeof u === "number" && typeof t === "number" && t > 0)
                                return (u / t) * 100
                            return 0
                        }
                        stale: client.snapshotStates["system_monitor"] === "stale"
                    }

                    MetricBar {
                        label: "Swap"
                        iconName: "swap"
                        value: {
                            const u = metricCard.field("swap.used")
                            const t = metricCard.field("swap.total")
                            if (typeof u === "number" && typeof t === "number" && t > 0)
                                return Format.formatBytes(u) + " / " + Format.formatBytes(t)
                            return "\u2014"
                        }
                        percent: {
                            const u = metricCard.field("swap.used")
                            const t = metricCard.field("swap.total")
                            if (typeof u === "number" && typeof t === "number" && t > 0)
                                return (u / t) * 100
                            return 0
                        }
                        stale: client.snapshotStates["system_monitor"] === "stale"
                    }

                    Text {
                        text: "STORAGE"
                        color: "#8b93a3"
                        font.pixelSize: 10
                        font.weight: Font.DemiBold
                        font.letterSpacing: 0.8
                        Layout.fillWidth: true
                        Layout.topMargin: 6
                    }

                    MetricBar {
                        label: "Disk"
                        iconName: "disk"
                        value: {
                            const u = metricCard.field("disk.used")
                            const t = metricCard.field("disk.total")
                            if (typeof u === "number" && typeof t === "number" && t > 0)
                                return Format.formatBytes(u) + " / " + Format.formatBytes(t)
                            return "\u2014"
                        }
                        detail: {
                            const r = metricCard.field("disk.read_bytes_sec")
                            const w = metricCard.field("disk.write_bytes_sec")
                            if (typeof r !== "number" || typeof w !== "number")
                                return ""
                            return "Read " + Format.formatBytes(r) + "/s   Write " + Format.formatBytes(w) + "/s"
                        }
                        percent: {
                            const u = metricCard.field("disk.used")
                            const t = metricCard.field("disk.total")
                            if (typeof u === "number" && typeof t === "number" && t > 0)
                                return (u / t) * 100
                            return 0
                        }
                        stale: client.snapshotStates["system_monitor"] === "stale"
                    }

                    Text {
                        text: "NETWORK"
                        color: "#8b93a3"
                        font.pixelSize: 10
                        font.weight: Font.DemiBold
                        font.letterSpacing: 0.8
                        Layout.fillWidth: true
                        Layout.topMargin: 6
                    }

                    MetricBar {
                        label: "Traffic"
                        iconName: "network"
                        showTrack: false
                        value: {
                            const rx = metricCard.field("network.rx_bytes_sec")
                            const tx = metricCard.field("network.tx_bytes_sec")
                            if (typeof rx === "number" && typeof tx === "number")
                                return "↓ " + Format.formatBytes(rx) + "/s"
                            return "\u2014"
                        }
                        detail: {
                            const tx = metricCard.field("network.tx_bytes_sec")
                            if (typeof tx !== "number")
                                return ""
                            return "↑ " + Format.formatBytes(tx) + "/s"
                        }
                        percent: 0
                        stale: client.snapshotStates["system_monitor"] === "stale"
                    }

                    Text {
                        text: "POWER"
                        color: "#8b93a3"
                        font.pixelSize: 10
                        font.weight: Font.DemiBold
                        font.letterSpacing: 0.8
                        Layout.fillWidth: true
                        Layout.topMargin: 6
                        visible: {
                            const b = metricCard.field("battery")
                            return !!b && b.present
                        }
                    }

                    MetricBar {
                        label: "Battery"
                        iconName: "battery"
                        visible: {
                            const b = metricCard.field("battery")
                            return !!b && b.present
                        }
                        value: {
                            const b = metricCard.field("battery")
                            if (!b || !b.present)
                                return "\u2014"
                            return Format.formatPercent(b.percent)
                        }
                        detail: {
                            const b = metricCard.field("battery")
                            if (!b || !b.present)
                                return ""
                            let s = b.state || "Unknown"
                            if (typeof b.power_watts === "number" && b.power_watts > 0)
                                s += "  ·  " + b.power_watts.toFixed(1) + " W"
                            return s
                        }
                        percent: {
                            const b = metricCard.field("battery")
                            if (!b || !b.present)
                                return 0
                            return b.percent
                        }
                        stale: client.snapshotStates["system_monitor"] === "stale"
                    }
                }

                Flow {
                    id: modules
                    Layout.fillWidth: true
                    Layout.preferredHeight: childrenRect.height
                    visible: client.modules.length > 0
                    spacing: 6

                    Repeater {
                        model: client.modules

                        delegate: Rectangle {
                            required property string modelData

                            width: chipLabel.implicitWidth + 22
                            height: 22
                            radius: 11
                            color: "#242830"
                            border.width: 1
                            border.color: "#343a46"

                            Rectangle {
                                id: dot
                                width: 6
                                height: 6
                                radius: 3
                                anchors.left: parent.left
                                anchors.leftMargin: 8
                                anchors.verticalCenter: parent.verticalCenter
                                color: {
                                    const state = client.subscriptions[parent.modelData]
                                    if (state === "subscribed")
                                        return "#7dcea0"
                                    if (state === "pending")
                                        return "#e6c07b"
                                    if (state === "denied" || state === "unavailable" || state === "failed")
                                        return "#f07178"
                                    return "#4b5262"
                                }
                            }

                            Text {
                                id: chipLabel
                                anchors.left: dot.right
                                anchors.leftMargin: 6
                                anchors.verticalCenter: parent.verticalCenter
                                text: parent.modelData
                                color: "#d7dde6"
                                font.pixelSize: 10
                            }
                        }
                    }
                }

                Text {
                    Layout.fillWidth: true
                    visible: client.modules.length === 0
                    text: client.backendConnected
                        ? "No modules reported."
                        : "Module data will appear after the handshake."
                    color: "#8b93a3"
                    font.pixelSize: 11
                    wrapMode: Text.WordWrap
                }

                RowLayout {
                    Layout.fillWidth: true
                    spacing: 8

                    ActionButton {
                        icon: "retry"
                        visible: !client.backendConnected
                        enabled: client.backendUnavailable || client.connecting
                        onActivated: client.retryNow()
                    }

                    ActionButton {
                        icon: "refresh"
                        enabled: client.backendConnected
                        onActivated: client.refreshHandshake()
                    }

                    Item { Layout.fillWidth: true }
                }
            }
        }
    }
}
