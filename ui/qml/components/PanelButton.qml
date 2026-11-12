import QtQuick
import QtQuick.Controls

Button {
    id: root
    property string description: text
    focusPolicy: Qt.StrongFocus
    Keys.onReturnPressed: clicked()
    Keys.onEnterPressed: clicked()
    Accessible.name: description
    implicitHeight: 32
    implicitWidth: Math.max(32, label.implicitWidth + 20)
    padding: 8
    ToolTip.visible: hovered
    ToolTip.delay: 600
    ToolTip.text: description
    background: Rectangle {
        radius: 9
        color: root.down ? Qt.alpha(Theme.accent, 0.22) : root.hovered ? Theme.surface : "transparent"
        border.color: root.visualFocus ? Theme.accent : "transparent"
        border.width: 2
    }
    contentItem: Text {
        id: label
        text: root.text
        color: root.enabled ? Theme.accent : Theme.muted
        font.pixelSize: 12
        horizontalAlignment: Text.AlignHCenter
        verticalAlignment: Text.AlignVCenter
    }
}
