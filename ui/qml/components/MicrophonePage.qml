import QtQuick
import QtQuick.Layouts

ColumnLayout {
    id: root
    property var snapshot: ({
            inputs: [],
            recordings: []
        })
    property bool ready: false
    property bool pending: false
    property var resolveIcon: function (name) {
        return "";
    }
    readonly property var microphones: (snapshot.inputs || []).filter(input => !input.monitor)
    readonly property var currentInput: microphones.find(input => input.name === snapshot.default_input) || null
    signal volumeRequested(string input, int volume)
    signal muteRequested(string input, bool muted)
    signal inputRequested(string input)
    spacing: 14
    Text {
        text: "DEFAULT MICROPHONE"
        color: Theme.muted
        font.pixelSize: 9
        font.letterSpacing: 1
    }
    DevicePicker {
        objectName: "inputSelector"
        Layout.fillWidth: true
        devices: root.microphones
        selectedDevice: root.snapshot.default_input || ""
        enabled: root.ready && count > 0
        pending: root.pending
        emptyText: "No microphones connected"
        missingText: "Microphone unavailable"
        Accessible.name: "Default microphone input"
        onDeviceChosen: device => root.inputRequested(device)
    }
    Text {
        visible: root.currentInput === null
        Layout.fillWidth: true
        text: root.microphones.length > 0 ? "Choose a microphone to adjust its gain and mute." : "Connect a microphone and its controls will appear here."
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 11
    }
    MixerRow {
        objectName: "microphoneGain"
        Layout.fillWidth: true
        visible: root.currentInput !== null
        title: root.currentInput ? root.currentInput.description : "Microphone"
        subtitle: "Microphone gain"
        iconSource: root.resolveIcon("audio-input-microphone")
        showOutputPicker: false
        controlsEnabled: root.ready && root.currentInput !== null
        pending: root.pending
        level: root.currentInput ? root.currentInput.volume : 0
        muted: root.currentInput ? root.currentInput.muted : false
        onVolumeRequested: volume => {
            if (root.currentInput)
                root.volumeRequested(root.currentInput.name, volume);
        }
        onMuteRequested: muted => {
            if (root.currentInput)
                root.muteRequested(root.currentInput.name, muted);
        }
    }
    Text {
        text: "RECORDING APPS"
        color: Theme.muted
        font.pixelSize: 9
        font.letterSpacing: 1
        Layout.topMargin: 4
    }
    Text {
        Layout.fillWidth: true
        text: root.ready ? "Apps with open recording streams and their current sources." : "Recording information is unavailable."
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 11
    }
    Text {
        Layout.fillWidth: true
        visible: (root.snapshot.recordings || []).length === 0
        text: "No recording streams reported."
        color: Theme.muted
        font.pixelSize: 12
    }
    Repeater {
        model: root.snapshot.recordings || []
        delegate: Rectangle {
            required property var modelData
            readonly property var source: (root.snapshot.inputs || []).find(input => input.id === modelData.input_id) || null
            objectName: "recordingApp"
            Layout.fillWidth: true
            implicitHeight: 66
            radius: 14
            color: Qt.alpha(Theme.surface, 0.55)
            border.color: Theme.border
            RowLayout {
                anchors.fill: parent
                anchors.margins: 12
                spacing: 10
                Rectangle {
                    width: 30
                    height: 30
                    radius: 9
                    color: Qt.alpha(Theme.accent, 0.12)
                    Image {
                        id: icon
                        anchors.centerIn: parent
                        width: 22
                        height: 22
                        source: root.resolveIcon(modelData.icon || "")
                        sourceSize.width: 32
                        sourceSize.height: 32
                    }
                    Text {
                        anchors.centerIn: parent
                        visible: icon.status !== Image.Ready
                        text: (modelData.name || "?").slice(0, 1).toUpperCase()
                        color: Theme.accent
                    }
                }
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 3
                    Text {
                        text: modelData.name
                        textFormat: Text.PlainText
                        color: Theme.foreground
                        font.pixelSize: 12
                        font.weight: Font.DemiBold
                        Layout.fillWidth: true
                        elide: Text.ElideRight
                    }
                    Text {
                        objectName: "recordingSource"
                        Layout.fillWidth: true
                        text: source ? (source.monitor ? "System audio · " : "") + source.description : "Input unavailable"
                        textFormat: Text.PlainText
                        color: Theme.muted
                        font.pixelSize: 10
                        elide: Text.ElideRight
                    }
                }
                Text {
                    text: root.ready ? (modelData.paused ? "Paused" : "Open") : "Cached"
                    color: Theme.muted
                    font.pixelSize: 10
                }
            }
        }
    }
}
