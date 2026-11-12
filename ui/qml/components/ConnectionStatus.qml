import QtQuick

// Compact connection indicator used in the Omnibar header: a small colored
// dot with an optional short label. No pill background, no border chrome.
Item {
    id: root

    property string status: "connecting" // connected, connecting, unavailable
    property bool label: true

    readonly property bool connected: status === "connected"
    readonly property bool connecting: status === "connecting"
    readonly property bool unavailable: status === "unavailable"

    readonly property color statusColor: connected
        ? "#4ade80"
        : connecting
            ? "#fbbf24"
            : "#f87171"

    implicitWidth: (label ? content.implicitWidth : dotSize) + (label ? 8 : 0)
    implicitHeight: dotSize

    property real dotSize: 7

    Row {
        id: content
        anchors.verticalCenter: parent.verticalCenter
        spacing: 6

        Rectangle {
            width: root.dotSize
            height: root.dotSize
            radius: root.dotSize / 2
            color: root.statusColor
        }

        Text {
            visible: root.label
            anchors.verticalCenter: parent.verticalCenter
            text: root.connected
                ? "Connected"
                : root.connecting
                    ? "Connecting…"
                    : "Unavailable"
            color: root.statusColor
            font.pixelSize: 11
            font.weight: Font.Medium
        }
    }
}
