import QtQuick

// Compact connection indicator used in the Omnibar header.
Rectangle {
    id: root

    property string status: "connecting"

    readonly property bool connected: status === "connected"
    readonly property bool connecting: status === "connecting"

    readonly property color statusColor: connected
        ? "#4ade80"
        : connecting
            ? "#fbbf24"
            : "#f87171"

    readonly property string statusLabel: connected
        ? "Connected"
        : connecting
            ? "Connecting\u2026"
            : "Backend unavailable"

    implicitWidth: content.implicitWidth + 22
    implicitHeight: 28
    radius: height / 2
    color: Qt.rgba(statusColor.r, statusColor.g, statusColor.b, 0.12)
    border.width: 1
    border.color: Qt.rgba(statusColor.r, statusColor.g, statusColor.b, 0.35)

    Row {
        id: content
        anchors.centerIn: parent
        spacing: 7

        Rectangle {
            width: 7
            height: 7
            radius: 3.5
            anchors.verticalCenter: parent.verticalCenter
            color: root.statusColor
        }

        Text {
            anchors.verticalCenter: parent.verticalCenter
            text: root.statusLabel
            color: root.statusColor
            font.pixelSize: 12
            font.weight: Font.Medium
        }
    }
}
