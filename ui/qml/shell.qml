import QtQuick
import QtQuick.Layouts
import Quickshell
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
            top: 18
            left: 18
        }

        implicitWidth: 460
        implicitHeight: 380
        color: "transparent"
        exclusionMode: ExclusionMode.Ignore

        Rectangle {
            anchors.fill: parent
            radius: 14
            color: "#14161b"
            border.width: 1
            border.color: "#2a2e37"

            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 18
                spacing: 14

                RowLayout {
                    Layout.fillWidth: true
                    spacing: 12

                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: 2

                        Text {
                            text: "Omnibar"
                            color: "#f2f4f8"
                            font.pixelSize: 20
                            font.weight: Font.DemiBold
                        }

                        Text {
                            text: "Control center"
                            color: "#8b93a3"
                            font.pixelSize: 12
                        }
                    }

                    ConnectionStatus {
                        Layout.alignment: Qt.AlignTop
                        status: client.status
                    }
                }

                Rectangle {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 1
                    color: "#252a33"
                }

                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 4

                    Text {
                        Layout.fillWidth: true
                        text: {
                            if (client.backendConnected) {
                                const version = client.backendVersion || "unknown version"
                                return "Backend " + version + " is ready."
                            }
                            if (client.connecting)
                                return "Attempting to connect to the backend\u2026"
                            return "Waiting for the backend to become available."
                        }
                        color: "#c7ccd6"
                        font.pixelSize: 13
                        wrapMode: Text.WordWrap
                    }

                    Text {
                        Layout.fillWidth: true
                        visible: !client.backendConnected
                        text: client.lastError.length > 0
                            ? client.lastError
                            : "Start the backend; Omnibar will reconnect automatically."
                        color: "#8b93a3"
                        font.pixelSize: 11
                        elide: Text.ElideRight
                    }

                    Text {
                        visible: client.backendUnavailable
                        text: "Automatic retry in " + client.retryDelaySeconds + "s"
                        color: "#8b93a3"
                        font.pixelSize: 11
                    }
                }

                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 8

                    RowLayout {
                        Layout.fillWidth: true

                        Text {
                            text: "Modules"
                            color: "#8b93a3"
                            font.pixelSize: 11
                            font.weight: Font.Medium
                            font.letterSpacing: 0.8
                        }

                        Item {
                            Layout.fillWidth: true
                        }

                        Text {
                            visible: !client.backendConnected && client.hasCachedData
                            text: "cached"
                            color: "#6f7787"
                            font.pixelSize: 10
                            font.italic: true
                        }
                    }

                    // system_monitor card: live CPU, memory, and swap readings.
                    ModuleCard {
                        Layout.fillWidth: true
                        module: "system_monitor"
                        state: client.snapshotStates["system_monitor"] || "pending"
                        detail: {
                            const snap = client.snapshots["system_monitor"]
                            if (!snap || !snap.sampled_at)
                                return ""
                            return "Sampled " + Format.formatSampledAt(snap.sampled_at)
                        }

                        ColumnLayout {
                            spacing: 6
                            Layout.fillWidth: true

                            MetricBar {
                                Layout.fillWidth: true
                                label: "CPU"
                                value: {
                                    const cpu = client.snapshots["system_monitor"]?.cpu
                                    if (!cpu)
                                        return "\u2014"
                                    return Format.formatPercent(cpu.percent) + "  (" + (cpu.cores ? cpu.cores.length : 0) + " cores)"
                                }
                                percent: client.snapshots["system_monitor"]?.cpu?.percent || 0
                                stale: client.snapshotStates["system_monitor"] === "stale"
                            }

                            MetricBar {
                                Layout.fillWidth: true
                                label: "Memory"
                                value: {
                                    const mem = client.snapshots["system_monitor"]?.memory
                                    if (!mem)
                                        return "\u2014"
                                    return Format.formatBytes(mem.used) + " / " + Format.formatBytes(mem.total)
                                }
                                percent: {
                                    const mem = client.snapshots["system_monitor"]?.memory
                                    if (!mem || mem.total === 0)
                                        return 0
                                    return (mem.used / mem.total) * 100
                                }
                                stale: client.snapshotStates["system_monitor"] === "stale"
                            }

                            MetricBar {
                                Layout.fillWidth: true
                                label: "Swap"
                                value: {
                                    const swap = client.snapshots["system_monitor"]?.swap
                                    if (!swap)
                                        return "\u2014"
                                    return Format.formatBytes(swap.used) + " / " + Format.formatBytes(swap.total)
                                }
                                percent: {
                                    const swap = client.snapshots["system_monitor"]?.swap
                                    if (!swap || swap.total === 0)
                                        return 0
                                    return (swap.used / swap.total) * 100
                                }
                                stale: client.snapshotStates["system_monitor"] === "stale"
                            }

                            MetricBar {
                                Layout.fillWidth: true
                                label: "Disk"
                                value: {
                                    const disk = client.snapshots["system_monitor"]?.disk
                                    if (!disk || !disk.total)
                                        return "\u2014"
                                    let str = Format.formatBytes(disk.used) + " / " + Format.formatBytes(disk.total)
                                    if (disk.read_bytes_sec > 0 || disk.write_bytes_sec > 0) {
                                        str += "  (R: " + Format.formatBytes(disk.read_bytes_sec) + "/s W: " + Format.formatBytes(disk.write_bytes_sec) + "/s)"
                                    }
                                    return str
                                }
                                percent: {
                                    const disk = client.snapshots["system_monitor"]?.disk
                                    if (!disk || disk.total === 0)
                                        return 0
                                    return (disk.used / disk.total) * 100
                                }
                                stale: client.snapshotStates["system_monitor"] === "stale"
                            }
                        }
                    }

                    // Module list chips: one chip per module reported at
                    // handshake, with a per-module subscription dot.
                    Flow {
                        Layout.fillWidth: true
                        Layout.preferredHeight: childrenRect.height
                        spacing: 8

                        Repeater {
                            model: client.modules

                            delegate: Rectangle {
                                required property string modelData

                                width: moduleName.implicitWidth + subscriptionDot.width + 28
                                height: 26
                                radius: 6
                                color: "#1d222a"
                                border.width: 1
                                border.color: "#303744"
                                opacity: client.backendConnected ? 1 : 0.55

                                Rectangle {
                                    id: subscriptionDot
                                    width: 6
                                    height: 6
                                    radius: 3
                                    anchors.left: parent.left
                                    anchors.leftMargin: 9
                                    anchors.verticalCenter: parent.verticalCenter
                                    color: {
                                        const state = client.subscriptions[parent.modelData]
                                        if (state === "subscribed")
                                            return "#4ade80"
                                        if (state === "pending")
                                            return "#fbbf24"
                                        if (state === "denied" || state === "unavailable" || state === "failed")
                                            return "#f87171"
                                        return "#4b5262"
                                    }
                                }

                                Text {
                                    id: moduleName
                                    anchors.left: subscriptionDot.right
                                    anchors.leftMargin: 7
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: parent.modelData
                                    color: "#dfe4ec"
                                    font.pixelSize: 11
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
                        color: "#6f7787"
                        font.pixelSize: 11
                    }
                }

                Item {
                    Layout.fillHeight: true
                }

                RowLayout {
                    Layout.fillWidth: true
                    spacing: 8

                    ActionButton {
                        label: "Retry now"
                        visible: !client.backendConnected
                        onActivated: client.retryNow()
                    }

                    ActionButton {
                        label: "Refresh handshake"
                        enabled: client.backendConnected
                        onActivated: client.refreshHandshake()
                    }

                    Item {
                        Layout.fillWidth: true
                    }

                    Text {
                        text: client.protocolVersion
                        color: "#6f7787"
                        font.pixelSize: 10
                    }
                }

                Text {
                    Layout.fillWidth: true
                    text: client.socketPath.length > 0
                        ? client.socketPath
                        : "XDG_RUNTIME_DIR is not set"
                    color: "#5c6473"
                    font.pixelSize: 10
                    elide: Text.ElideMiddle
                }
            }
        }
    }
}
