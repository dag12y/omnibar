import QtQuick
import QtQuick.Layouts
import QtQuick.Controls

AbstractButton {
    id: root
    property string label: ""
    property string iconName: "cpu"
    property string value: "—"
    property string detail: ""
    property var samples: []
    property bool stale: false
    signal activated
    implicitHeight: 150
    padding: 16
    focusPolicy: Qt.StrongFocus
    Keys.onReturnPressed: clicked()
    Keys.onEnterPressed: clicked()
    Accessible.name: label + ": " + value + ". Show details"
    onClicked: activated()
    ToolTip.visible: hovered
    ToolTip.delay: 600
    ToolTip.text: "Show " + label.toLowerCase() + " details"
    background: Rectangle {
        radius: 18
        color: Qt.alpha(Theme.surface, root.hovered ? 0.85 : 0.55)
        border.color: root.visualFocus ? Theme.accent : root.hovered ? Qt.alpha(Theme.accent, 0.5) : Theme.border
        border.width: root.visualFocus ? 2 : 1
    }
    contentItem: ColumnLayout {
        spacing: 6
        RowLayout {
            MetricIcon {
                name: root.iconName
                size: 16
                color: Theme.accent
            }
            Text {
                text: root.label
                color: Theme.muted
                font.pixelSize: 12
                Layout.fillWidth: true
            }
            Text {
                text: "↗"
                color: Theme.muted
                font.pixelSize: 12
            }
        }
        Text {
            text: root.value
            color: Theme.foreground
            font.pixelSize: 30
            font.weight: Font.DemiBold
            opacity: root.stale ? 0.5 : 1
        }
        Text {
            text: root.detail
            color: Theme.muted
            font.pixelSize: 10
            elide: Text.ElideRight
            Layout.fillWidth: true
        }
        Sparkline {
            samples: root.samples
            Layout.fillWidth: true
            Layout.fillHeight: true
            opacity: root.stale ? 0.35 : 1
        }
    }
}
