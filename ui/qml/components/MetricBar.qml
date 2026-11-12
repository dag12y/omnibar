import QtQuick
import QtQuick.Layouts

// One metric row: icon, label, right-aligned value, optional quieter
// detail line, and a thin track inside the row so it counts toward height.
Item {
    id: root

    property string label: ""
    property string iconName: ""
    property string value: "\u2014"
    property string detail: ""
    property real percent: 0
    property bool stale: false
    property bool showTrack: true

    Layout.fillWidth: true
    Layout.preferredHeight: implicitHeight

    implicitWidth: 240
    implicitHeight: (detail.length > 0 ? 46 : 32)

    RowLayout {
        id: row
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        spacing: 8

        MetricIcon {
            name: root.iconName
            size: 15
            color: root.stale ? Theme.muted : Theme.foreground
            Layout.alignment: Qt.AlignVCenter
        }

        ColumnLayout {
            Layout.fillWidth: true
            spacing: 1

            RowLayout {
                Layout.fillWidth: true
                spacing: 8

                Text {
                    text: root.label
                    color: root.stale ? Theme.muted : Theme.foreground
                    font.pixelSize: 12
                    font.weight: Font.Medium
                }

                Item { Layout.fillWidth: true }

                Text {
                    text: root.value
                    color: root.stale ? Theme.muted : Theme.foreground
                    font.pixelSize: 12
                    font.weight: Font.DemiBold
                    horizontalAlignment: Text.AlignRight
                    elide: Text.ElideRight
                    Layout.maximumWidth: 210
                }
            }

            Text {
                visible: root.detail.length > 0
                text: root.detail
                color: Theme.muted
                font.pixelSize: 10
                elide: Text.ElideRight
                Layout.fillWidth: true
            }
        }
    }

    Rectangle {
        visible: root.showTrack
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        height: 3
        radius: 1.5
        color: Theme.border

        Rectangle {
            width: parent.width * Math.max(0, Math.min(1, root.percent / 100))
            height: parent.height
            radius: 1.5
            color: root.percent > 80 ? Theme.urgent : Theme.accent
            opacity: root.stale ? 0.45 : 1
            Behavior on width { NumberAnimation { duration: 240 } }
        }
    }
}
