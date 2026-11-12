import QtQuick
import QtQuick.Layouts

// A quiet frosted card. Children are laid out in a column and the card
// grows to fit them. Do not give it a fixed height.
Rectangle {
    id: root

    property bool stale: false
    default property alias content: stack.data

    implicitWidth: 280
    implicitHeight: stack.implicitHeight + 24

    color: "#1c1e24"
    radius: 14
    border.width: 1
    border.color: stale ? "#3a4150" : "#2c313b"

    ColumnLayout {
        id: stack
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: 12
        spacing: 8
    }
}
