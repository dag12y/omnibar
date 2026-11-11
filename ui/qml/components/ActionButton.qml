import QtQuick

// Minimal flat button. Disabled buttons stay visible and muted so the
// UI clearly shows which backend-dependent actions are unavailable.
Rectangle {
    id: root

    property string label: ""
    property bool primary: false

    signal activated()

    implicitWidth: labelText.implicitWidth + 28
    implicitHeight: 32
    radius: 8

    color: {
        if (!enabled)
            return "#1b1f26"
        if (primary)
            return mouseArea.containsMouse ? "#3b82f6" : "#2563eb"
        return mouseArea.containsMouse ? "#2d333e" : "#242a33"
    }

    border.width: 1
    border.color: {
        if (!enabled)
            return "#232830"
        return primary ? "#3b82f6" : "#343b47"
    }

    opacity: enabled ? 1 : 0.5

    Text {
        id: labelText
        anchors.centerIn: parent
        text: root.label
        color: root.enabled ? "#f2f4f8" : "#7b8291"
        font.pixelSize: 12
        font.weight: Font.Medium
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
