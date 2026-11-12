import QtQuick
import QtQuick.Layouts

Rectangle {
    id: root
    property bool active: false
    property bool connected: false
    property bool available: false
    property bool denied: false
    property bool busy: false
    property bool clockRunning: true
    property string expiresAt: ""
    property string errorText: ""
    property double now: Date.now()
    readonly property int remaining: Math.max(0, Math.ceil((Date.parse(expiresAt) - now) / 1000))
    signal startRequested(int seconds)
    signal stopRequested
    implicitHeight: content.implicitHeight + 28
    radius: 16
    color: Qt.alpha(Theme.accent, active ? 0.12 : 0.04)
    border.color: active ? Qt.alpha(Theme.accent, 0.4) : Theme.border
    Timer {
        interval: 1000
        running: root.active && root.clockRunning
        repeat: true
        triggeredOnStart: true
        onTriggered: root.now = Date.now()
    }
    onExpiresAtChanged: now = Date.now()
    ColumnLayout {
        id: content
        x: 14
        y: 14
        width: parent.width - 28
        spacing: 8
        RowLayout {
            Layout.fillWidth: true
            Text {
                text: "☕"
                color: Theme.accent
                font.pixelSize: 18
            }
            Text {
                text: "Keep awake"
                color: Theme.foreground
                font.pixelSize: 12
                font.weight: Font.DemiBold
                Layout.fillWidth: true
            }
            Text {
                visible: root.active
                text: root.connected ? "ACTIVE" : "UNCONFIRMED"
                color: Theme.accent
                font.pixelSize: 9
                font.letterSpacing: 0.5
            }
        }
        Text {
            Layout.fillWidth: true
            wrapMode: Text.WordWrap
            color: root.errorText ? Theme.urgent : Theme.muted
            font.pixelSize: 11
            text: root.errorText || (!root.connected ? (root.active ? "Connection lost. Your session may still be running." : "Connect to start a session.") : root.denied ? "Enable power.inhibit in your Omnibar config." : !root.available ? "Keep awake is unavailable." : root.busy ? "Updating session…" : root.active ? (Math.floor(root.remaining / 60) + ":" + String(root.remaining % 60).padStart(2, "0") + " remaining · idle and sleep inhibited") : "Prevent idle and sleep for a timed session.")
        }
        RowLayout {
            Layout.fillWidth: true
            spacing: 4
            Repeater {
                model: [15, 30, 60]
                PanelButton {
                    required property int modelData
                    objectName: "start" + modelData
                    visible: !root.active
                    enabled: root.connected && root.available && !root.busy
                    text: modelData + " min"
                    description: "Keep awake for " + modelData + " minutes"
                    onClicked: root.startRequested(modelData * 60)
                }
            }
            PanelButton {
                objectName: "stopSession"
                visible: root.active
                enabled: root.connected && root.available && !root.busy
                text: "Stop session"
                description: "Stop keeping the system awake"
                onClicked: root.stopRequested()
            }
            Item {
                Layout.fillWidth: true
            }
        }
    }
}
