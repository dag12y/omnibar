import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    property var snapshot: ({
            entries: [],
            collecting: false,
            state: "paused",
            can_copy: false
        })
    property string state: "pending"
    property string errorText: ""
    property bool busy: false
    readonly property bool ready: state === "ready"
    signal collectionRequested(bool collecting)
    signal copyRequested(string entryId)
    signal deleteRequested(string entryId)
    signal clearRequested
    signal refreshRequested
    spacing: 14
    ListModel {
        id: entries
    }
    function syncRows() {
        const query = search.text.toLowerCase();
        const wanted = (snapshot.entries || []).filter(entry => entry.text.toLowerCase().includes(query));
        for (let i = entries.count - 1; i >= 0; i--)
            if (!wanted.some(entry => entry.id === entries.get(i).entryId))
                entries.remove(i);
        for (let i = 0; i < wanted.length; i++) {
            const entry = wanted[i];
            let current = -1;
            for (let j = i; j < entries.count; j++)
                if (entries.get(j).entryId === entry.id) {
                    current = j;
                    break;
                }
            const row = {
                entryId: entry.id,
                preview: Array.from(entry.text).slice(0, 500).join(""),
                copiedAt: entry.copied_at
            };
            if (current < 0)
                entries.insert(i, row);
            else {
                if (current !== i)
                    entries.move(current, i, 1);
                entries.set(i, row);
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
                text: "Clipboard"
                color: Theme.foreground
                font.pixelSize: 17
                font.weight: Font.DemiBold
            }
            Text {
                text: "Copied text, kept for this session"
                color: Theme.muted
                font.pixelSize: 11
            }
        }
        Item {
            Layout.fillWidth: true
        }
        PanelButton {
            text: "Refresh ↻"
            description: "Refresh clipboard history"
            enabled: !root.busy && root.state !== "offline"
            onClicked: root.refreshRequested()
        }
    }
    Text {
        Layout.fillWidth: true
        visible: !root.ready || root.errorText.length > 0 || !!root.snapshot.error
        text: root.state === "denied" ? "Enable clipboard.read in your Omnibar config and restart the backend." : root.state === "disabled" ? "Clipboard history is disabled in your config." : root.state === "offline" ? "Backend unavailable. Reconnecting…" : root.errorText || root.snapshot.error || "Loading clipboard history…"
        color: Theme.muted
        font.pixelSize: 12
        wrapMode: Text.WordWrap
        textFormat: Text.PlainText
    }
    Rectangle {
        Layout.fillWidth: true
        implicitHeight: collection.implicitHeight + 24
        radius: 14
        color: Qt.alpha(Theme.accent, 0.08)
        border.color: Theme.border
        ColumnLayout {
            id: collection
            x: 12
            y: 12
            width: parent.width - 24
            spacing: 6
            Text {
                text: root.snapshot.collecting ? (root.snapshot.state === "retrying" ? "Reconnecting clipboard watcher" : "Collecting copied text") : "History collection is paused"
                color: Theme.foreground
                font.pixelSize: 12
                font.weight: Font.DemiBold
                Layout.fillWidth: true
                wrapMode: Text.WordWrap
            }
            Text {
                text: "Kept in memory until Omnibar restarts. Collection continues when the panel is hidden."
                color: Theme.muted
                font.pixelSize: 11
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }
            RowLayout {
                PanelButton {
                    objectName: "collectionToggle"
                    text: root.snapshot.collecting ? "Pause" : "Start history"
                    description: root.snapshot.collecting ? "Pause clipboard collection" : "Start collecting copied text in memory"
                    enabled: root.ready
                    onClicked: root.collectionRequested(!root.snapshot.collecting)
                }
                Item {
                    Layout.fillWidth: true
                }
                PanelButton {
                    objectName: "clearHistory"
                    text: "Clear history"
                    enabled: root.ready && (root.snapshot.entries || []).length > 0
                    onClicked: root.clearRequested()
                }
            }
        }
    }
    TextField {
        id: search
        objectName: "clipboardSearch"
        Layout.fillWidth: true
        enabled: root.ready
        placeholderText: "Search copied text…"
        Accessible.name: "Search clipboard history"
        color: Theme.foreground
        placeholderTextColor: Theme.muted
        selectionColor: Theme.accent
        selectedTextColor: Theme.background
        implicitHeight: 38
        leftPadding: 12
        rightPadding: 12
        background: Rectangle {
            radius: 10
            color: Theme.surface
            border.color: search.activeFocus ? Theme.accent : Theme.border
        }
        onTextChanged: root.syncRows()
    }
    Text {
        visible: root.ready && entries.count === 0
        text: search.text ? "No matching copied text." : "Copied text will appear here after you start history."
        Layout.fillWidth: true
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 12
    }
    Repeater {
        model: entries
        delegate: Rectangle {
            required property string entryId
            required property string preview
            required property string copiedAt
            objectName: "history-" + entryId
            Layout.fillWidth: true
            implicitHeight: item.implicitHeight + 24
            radius: 14
            color: Qt.alpha(Theme.surface, 0.55)
            border.color: Theme.border
            ColumnLayout {
                id: item
                x: 12
                y: 12
                width: parent.width - 24
                spacing: 8
                Text {
                    text: preview
                    textFormat: Text.PlainText
                    Layout.fillWidth: true
                    wrapMode: Text.WrapAnywhere
                    maximumLineCount: 3
                    elide: Text.ElideRight
                    color: Theme.foreground
                    font.pixelSize: 12
                }
                RowLayout {
                    Layout.fillWidth: true
                    Text {
                        text: copiedAt ? Qt.formatTime(new Date(copiedAt), "hh:mm") : ""
                        color: Theme.muted
                        font.pixelSize: 10
                        Layout.fillWidth: true
                    }
                    PanelButton {
                        objectName: "copyHistory"
                        text: "Copy"
                        description: root.snapshot.can_copy ? "Copy this history item to the clipboard" : "Enable clipboard.write to copy history items"
                        enabled: root.ready && root.snapshot.can_copy === true
                        onClicked: root.copyRequested(entryId)
                    }
                    PanelButton {
                        objectName: "deleteHistory"
                        text: "Delete"
                        description: "Remove this history item"
                        enabled: root.ready
                        onClicked: root.deleteRequested(entryId)
                    }
                }
            }
        }
    }
}
