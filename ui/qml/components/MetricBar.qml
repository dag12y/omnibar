import QtQuick
import QtQuick.Layouts
import "format.js" as Format

// A horizontal metric bar: a label, a value, and a filled track that
// visualizes a percentage (0-100). Used by the system monitor card for
// CPU and memory readings.
Rectangle {
    id: root

    property string label: ""
    property string value: "\u2014"        // e.g. "4.2%" or "12.3 GB"
    property real percent: 0              // 0-100 fill of the track
    property bool stale: false            // dims the card when true

    implicitWidth: 200
    implicitHeight: 44

    color: "#1d222a"
    radius: 8
    border.width: 1
    border.color: stale ? "#343b47" : "#303744"

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 10
        spacing: 4

        RowLayout {
            spacing: 6

            Text {
                text: root.label
                color: "#8b93a3"
                font.pixelSize: 11
                font.weight: Font.Medium
                Layout.alignment: Qt.AlignLeft
            }

            Item {
                Layout.fillWidth: true
            }

            Text {
                id: valueText
                text: root.value
                color: root.stale ? "#7b8291" : "#dfe4ec"
                font.pixelSize: 12
                font.weight: Font.DemiBold
            }
        }

        Rectangle {
            Layout.fillWidth: true
            Layout.preferredHeight: 6
            radius: 3
            color: "#232830"

            Rectangle {
                id: fill
                width: parent.width * Math.max(0, Math.min(1, root.percent / 100))
                height: parent.height
                radius: 3
                color: root.percent > 80 ? "#f87171" : root.percent > 50 ? "#fbbf24" : "#4ade80"
                Behavior on width { NumberAnimation { duration: 300 } }
            }
        }
    }
}
