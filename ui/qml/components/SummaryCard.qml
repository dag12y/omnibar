import QtQuick
import QtQuick.Layouts

Rectangle {
    id: root
    property string label: ""
    property string iconName: "cpu"
    property string value: "—"
    property string detail: ""
    property var samples: []
    property bool stale: false
    signal activated()
    implicitHeight: 150
    radius: 18
    color: Qt.alpha(Theme.surface, mouse.containsMouse ? 0.85 : 0.55)
    border.color: mouse.containsMouse ? Qt.alpha(Theme.accent, 0.5) : Theme.border
    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 16
        spacing: 6
        RowLayout {
            MetricIcon { name: root.iconName; size: 16; color: Theme.accent }
            Text { text: root.label; color: Theme.muted; font.pixelSize: 12; Layout.fillWidth: true }
            Text { text: "↗"; color: Theme.muted; font.pixelSize: 12 }
        }
        Text { text: root.value; color: Theme.foreground; font.pixelSize: 30; font.weight: Font.DemiBold; opacity: root.stale ? 0.5 : 1 }
        Text { text: root.detail; color: Theme.muted; font.pixelSize: 10; elide: Text.ElideRight; Layout.fillWidth: true }
        Sparkline { samples: root.samples; Layout.fillWidth: true; Layout.fillHeight: true; opacity: root.stale ? 0.35 : 1 }
    }
    MouseArea { id: mouse; anchors.fill: parent; hoverEnabled: true; cursorShape: Qt.PointingHandCursor; onClicked: root.activated() }
}
