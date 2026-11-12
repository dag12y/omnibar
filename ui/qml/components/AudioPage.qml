import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    property var snapshot: ({
            outputs: [],
            streams: [],
            available: false
        })
    property string state: "pending"
    property string errorText: ""
    property bool busy: false
    property var resolveIcon: function (name) {
        return "";
    }
    readonly property bool ready: state === "ready" && snapshot.available === true
    signal volumeRequested(string key, int volume)
    signal muteRequested(string key, bool muted)
    signal outputRequested(string output)
    signal refreshRequested
    spacing: 14
    ListModel {
        id: rows
    }
    function syncRows() {
        const streams = snapshot.streams || [];
        for (let i = rows.count - 1; i >= 0; i--)
            if (!streams.some(stream => stream.key === rows.get(i).streamKey))
                rows.remove(i);
        for (let i = 0; i < streams.length; i++) {
            const stream = streams[i];
            let current = -1;
            for (let j = i; j < rows.count; j++)
                if (rows.get(j).streamKey === stream.key) {
                    current = j;
                    break;
                }
            const row = {
                streamKey: stream.key,
                title: stream.name,
                media: stream.media || "Playback",
                iconName: stream.icon || "",
                level: stream.volume,
                muted: stream.muted,
                paused: stream.paused,
                outputId: stream.output_id
            };
            if (current < 0)
                rows.insert(i, row);
            else {
                if (current !== i)
                    rows.move(current, i, 1);
                rows.set(i, row);
            }
        }
    }
    onSnapshotChanged: syncRows()
    Component.onCompleted: syncRows()
    RowLayout {
        Layout.fillWidth: true
        ColumnLayout {
            Layout.fillWidth: true
            Text {
                text: "Audio mixer"
                color: Theme.foreground
                font.pixelSize: 17
                font.weight: Font.DemiBold
            }
            Text {
                text: "Your apps. Your levels."
                color: Theme.muted
                font.pixelSize: 11
            }
        }
        Item {
            Layout.fillWidth: true
        }
        PanelButton {
            text: "Refresh ↻"
            description: "Refresh audio devices and streams"
            enabled: !root.busy && root.state !== "offline" && root.state !== "denied"
            onClicked: root.refreshRequested()
        }
    }
    Text {
        Layout.fillWidth: true
        visible: !root.ready || root.errorText.length > 0
        wrapMode: Text.WordWrap
        color: root.errorText.length > 0 ? Theme.urgent : Theme.muted
        font.pixelSize: 12
        text: root.state === "denied" ? "Enable audio.control in your Omnibar config and restart the backend." : root.state === "offline" ? "Backend unavailable. Reconnecting…" : root.errorText || (root.state === "pending" ? "Loading your audio devices…" : root.snapshot.error || "Audio service unavailable. Check that PipeWire and pipewire-pulse are running. Retrying…")
    }
    ColumnLayout {
        Layout.fillWidth: true
        spacing: 6
        visible: root.ready
        Text {
            text: "DEFAULT OUTPUT"
            color: Theme.muted
            font.pixelSize: 9
            font.letterSpacing: 1
        }
        ComboBox {
            id: outputs
            objectName: "outputSelector"
            implicitHeight: 38
            leftPadding: 12
            rightPadding: 32
            font.pixelSize: 12
            Layout.fillWidth: true
            enabled: root.ready && count > 0
            model: root.snapshot.outputs || []
            textRole: "description"
            valueRole: "name"
            currentIndex: {
                const items = root.snapshot.outputs || [];
                return items.findIndex(item => item.name === root.snapshot.default_output);
            }
            Accessible.name: "Default playback output"
            onActivated: index => root.outputRequested(root.snapshot.outputs[index].name)
            palette.text: Theme.foreground
            palette.buttonText: Theme.foreground
            palette.base: Theme.background
            palette.highlight: Theme.accent
            palette.highlightedText: Theme.background
            background: Rectangle {
                radius: 10
                color: Theme.surface
                border.color: outputs.visualFocus ? Theme.accent : Theme.border
            }
        }
        Text {
            text: "No playback outputs connected"
            visible: (root.snapshot.outputs || []).length === 0
            color: Theme.muted
            font.pixelSize: 11
        }
    }
    Text {
        text: "PLAYBACK APPS"
        visible: root.ready
        color: Theme.muted
        font.pixelSize: 9
        font.letterSpacing: 1
        Layout.topMargin: 4
    }
    Text {
        Layout.fillWidth: true
        visible: root.ready && rows.count === 0
        text: "Nothing playing yet. Start audio in an app and its controls will appear here."
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 12
    }
    Repeater {
        model: rows
        delegate: MixerRow {
            required property string streamKey
            required property string media
            required property string iconName
            required property string outputId
            required title
            required level
            required muted
            required paused
            Layout.fillWidth: true
            controlsEnabled: root.ready
            pending: root.busy
            iconSource: root.resolveIcon(iconName)
            subtitle: {
                const output = (root.snapshot.outputs || []).find(item => item.id === outputId);
                return media + (output ? " · " + output.description : "");
            }
            onVolumeRequested: volume => root.volumeRequested(streamKey, volume)
            onMuteRequested: muted => root.muteRequested(streamKey, muted)
        }
    }
}
