import QtQuick
import QtQuick.Layouts
import "format.js" as Format

// A module card: a titled card showing one module's live readings, with a
// connection status dot, a state badge (fresh / stale / denied /
// unavailable), and metric bars supplied by the caller. Used by the
// system_monitor card for CPU, memory, and swap.
Rectangle {
    id: root

    property string module: ""
    property string state: "pending"   // pending, fresh, stale, denied, unavailable, failed
    property string detail: ""         // extra line shown under the state badge

    readonly property bool connected: state === "fresh"
    readonly property bool stale: state === "stale"
    readonly property bool denied: state === "denied"
    readonly property bool unavailable: state === "unavailable" || state === "failed"

    readonly property color stateColor: connected
        ? "#4ade80"
        : stale
            ? "#fbbf24"
            : denied || unavailable
                ? "#f87171"
                : "#4b5262"

    readonly property string stateLabel: connected
        ? "Live"
        : stale
            ? "Stale"
            : denied
                ? "Permission denied"
                : unavailable
                    ? "Unavailable"
                    : "Pending"

    implicitWidth: 220
    implicitHeight: 200
    color: "#14161b"
    radius: 12
    border.width: 1
    border.color: root.stale ? "#343b47" : "#2a2e37"

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 14
        spacing: 10

        // Header: module name, status dot, state badge.
        RowLayout {
            Layout.fillWidth: true
            spacing: 8

            Text {
                id: moduleName
                text: root.module
                color: "#f2f4f8"
                font.pixelSize: 14
                font.weight: Font.DemiBold
                elide: Text.ElideRight
                Layout.fillWidth: true
            }

            Rectangle {
                width: 7
                height: 7
                radius: 3.5
                anchors.verticalCenter: parent.verticalCenter
                color: root.stateColor
            }

            Text {
                id: stateBadge
                text: root.stateLabel
                color: root.stateColor
                font.pixelSize: 10
                font.weight: Font.Medium
                visible: root.state !== "pending"
            }
        }

        Rectangle {
            Layout.fillWidth: true
            Layout.preferredHeight: 1
            color: "#252a33"
        }

        // Caller-provided content (metric bars, etc.) goes in contentItem.
        Item {
            id: contentItem
            Layout.fillWidth: true
            Layout.fillHeight: true
        }

        // Detail line (e.g. last sampled time or denial reason).
        Text {
            id: detailText
            text: root.detail
            color: root.stale ? "#7b8291" : root.denied || root.unavailable ? "#f87171" : "#8b93a3"
            font.pixelSize: 10
            font.italic: true
            wrapMode: Text.WordWrap
            visible: root.detail.length > 0
        }
    }
}
