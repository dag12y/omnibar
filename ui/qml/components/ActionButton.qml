import QtQuick

// Icon-only action button. No wide text label; the icon carries the meaning.
// Disabled buttons stay visible and muted.
Item {
    id: root

    property string icon: "refresh"   // passed to MetricIcon name
    property bool enabled: true
    property bool primary: false

    signal activated()

    implicitWidth: 32
    implicitHeight: 32

    readonly property color iconColor: enabled
        ? (primary ? "#2563eb" : "#dfe4ec")
        : "#7b8291"

    Rectangle {
        id: bg
        anchors.fill: parent
        radius: 8
        color: {
            if (!enabled)
                return "#1b1f26"
            if (mouseArea.containsMouse)
                return primary ? "#3b82f6" : "#2d333e"
            return primary ? "#2563eb" : "#242a33"
        }
        border.width: 1
        border.color: {
            if (!enabled)
                return "#232830"
            return primary ? "#3b82f6" : "#343b47"
        }
        opacity: enabled ? 1 : 0.5
    }

    MetricIcon {
        id: icon
        anchors.centerIn: parent
        name: root.icon
        size: 14
        color: root.iconColor
    }

    MouseArea {
        id: mouseArea
        anchors.fill: parent
        enabled: root.enabled
        hoverEnabled: true
        cursorShape: root.enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
        onClicked: root.activated()
    }
}
