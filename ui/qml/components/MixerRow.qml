import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Rectangle {
    id: root
    property string title: ""
    property string subtitle: ""
    property string iconSource: ""
    property var outputs: []
    property string outputName: ""
    property int level: 0
    property bool muted: false
    property bool paused: false
    property bool controlsEnabled: true
    property bool pending: false
    property bool showOutputPicker: true
    onPendingChanged: {
        if (!pending)
            syncLevel();
    }
    signal volumeRequested(int volume)
    signal muteRequested(bool muted)
    signal routeRequested(string output)
    implicitHeight: showOutputPicker ? 152 : 108
    radius: 16
    color: Qt.alpha(Theme.surface, 0.55)
    border.color: Theme.border
    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 14
        spacing: 6
        RowLayout {
            Layout.fillWidth: true
            Rectangle {
                width: 30
                height: 30
                radius: 9
                color: Qt.alpha(Theme.accent, 0.12)
                Image {
                    id: appIcon
                    anchors.centerIn: parent
                    width: 22
                    height: 22
                    source: root.iconSource
                    sourceSize.width: 32
                    sourceSize.height: 32
                }
                Text {
                    anchors.centerIn: parent
                    text: root.title.slice(0, 1).toUpperCase()
                    color: Theme.accent
                    visible: appIcon.status !== Image.Ready
                }
            }
            ColumnLayout {
                Layout.fillWidth: true
                spacing: 2
                Text {
                    text: root.title
                    textFormat: Text.PlainText
                    color: Theme.foreground
                    font.pixelSize: 12
                    font.weight: Font.DemiBold
                    Layout.fillWidth: true
                    elide: Text.ElideRight
                }
                Text {
                    text: (root.paused ? "Paused · " : "") + root.subtitle
                    textFormat: Text.PlainText
                    color: Theme.muted
                    font.pixelSize: 10
                    Layout.fillWidth: true
                    elide: Text.ElideRight
                }
            }
            PanelButton {
                objectName: "mute"
                enabled: root.controlsEnabled
                text: root.muted ? "Unmute" : "Mute"
                description: (root.muted ? "Unmute " : "Mute ") + root.title
                onClicked: root.muteRequested(!root.muted)
            }
        }
        RowLayout {
            Layout.fillWidth: true
            Slider {
                id: slider
                objectName: "volume"
                Layout.fillWidth: true
                from: 0
                to: 100
                stepSize: 1
                enabled: root.controlsEnabled
                focusPolicy: Qt.StrongFocus
                Accessible.name: root.title + " volume"
                property bool edited: false
                onMoved: {
                    edited = true;
                    if (!pressed) {
                        root.volumeRequested(Math.round(value));
                        edited = false;
                    }
                }
                onPressedChanged: {
                    if (!pressed && edited) {
                        root.volumeRequested(Math.round(value));
                        edited = false;
                    }
                }
                background: Rectangle {
                    x: slider.leftPadding
                    y: slider.topPadding + slider.availableHeight / 2 - height / 2
                    width: slider.availableWidth
                    height: 4
                    radius: 2
                    color: Theme.border
                    Rectangle {
                        width: parent.width * slider.visualPosition
                        height: parent.height
                        radius: 2
                        color: root.muted ? Theme.muted : Theme.accent
                    }
                }
                handle: Rectangle {
                    x: slider.leftPadding + slider.visualPosition * (slider.availableWidth - width)
                    y: slider.topPadding + slider.availableHeight / 2 - height / 2
                    width: 14
                    height: 14
                    radius: 7
                    color: Theme.foreground
                    border.width: slider.visualFocus ? 3 : 0
                    border.color: Theme.accent
                }
            }
            Text {
                Layout.preferredWidth: 40
                text: (slider.pressed ? Math.round(slider.value) : root.level) + "%"
                horizontalAlignment: Text.AlignRight
                color: root.muted ? Theme.muted : Theme.foreground
                font.pixelSize: 11
            }
        }
        RowLayout {
            visible: root.showOutputPicker
            Layout.fillWidth: true
            Text {
                text: "OUTPUT"
                color: Theme.muted
                font.pixelSize: 9
                font.letterSpacing: 0.5
            }
            OutputPicker {
                objectName: "streamOutputSelector"
                Layout.fillWidth: true
                enabled: root.controlsEnabled && count > 0
                outputs: root.outputs
                selectedOutput: root.outputName
                pending: root.pending
                Accessible.name: root.title + " playback output · " + root.subtitle
                onOutputChosen: output => root.routeRequested(output)
            }
        }
    }
    function syncLevel() {
        if (!slider.pressed && !pending)
            slider.value = Math.max(0, Math.min(100, level));
    }
    onLevelChanged: syncLevel()
    Component.onCompleted: syncLevel()
}
