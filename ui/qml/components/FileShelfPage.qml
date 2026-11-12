import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import "format.js" as Format

ColumnLayout {
    id: root
    property var snapshot: ({
            entries: [],
            can_copy: false
        })
    property string state: "pending"
    property string errorText: ""
    property bool busy: false
    readonly property bool ready: state === "ready"
    signal pathsRequested(var paths)
    signal removeRequested(string entryId)
    signal copyRequested(string entryId)
    signal folderRequested(string entryId)
    signal clearRequested
    signal refreshRequested
    spacing: 14
    ListModel {
        id: rows
    }
    function addPaths(paths) {
        if (!ready || busy || !paths || paths.length === 0)
            return false;
        root.pathsRequested(Array.from(paths, value => value.toString()));
        return true;
    }
    function handleDrop(event) {
        event.accepted = false;
        if (!event.hasUrls || !(event.supportedActions & Qt.CopyAction))
            return;
        if (addPaths(event.urls))
            event.accept(Qt.CopyAction);
    }
    function syncRows() {
        const query = search.text.toLowerCase();
        const wanted = (snapshot.entries || []).filter(entry => entry.path.toLowerCase().includes(query));
        for (let i = rows.count - 1; i >= 0; i--)
            if (!wanted.some(entry => entry.id === rows.get(i).entryId))
                rows.remove(i);
        for (let i = 0; i < wanted.length; i++) {
            const entry = wanted[i];
            let current = -1;
            for (let j = i; j < rows.count; j++)
                if (rows.get(j).entryId === entry.id) {
                    current = j;
                    break;
                }
            const row = {
                entryId: entry.id,
                filePath: entry.path,
                fileName: entry.name,
                directory: entry.directory === true,
                available: entry.available === true,
                fileSize: entry.size || 0
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
                text: "File shelf"
                color: Theme.foreground
                font.pixelSize: 17
                font.weight: Font.DemiBold
            }
            Text {
                text: "Files within reach, for this session"
                color: Theme.muted
                font.pixelSize: 11
            }
        }
        PanelButton {
            text: "Refresh ↻"
            enabled: !root.busy && root.state !== "offline"
            onClicked: root.refreshRequested()
        }
    }
    Text {
        Layout.fillWidth: true
        visible: !root.ready || root.errorText.length > 0
        text: root.state === "denied" ? "Enable filesystem.access in ~/.config/omnibar/config.toml and restart the backend." : root.state === "disabled" ? "File shelf is disabled in your config." : root.state === "offline" ? "Backend unavailable. Reconnecting…" : root.errorText || "Loading file shelf…"
        color: Theme.muted
        font.pixelSize: 12
        wrapMode: Text.Wrap
        textFormat: Text.PlainText
    }
    Rectangle {
        Layout.fillWidth: true
        implicitHeight: addBox.implicitHeight + 24
        radius: 14
        color: drop.containsDrag ? Qt.alpha(Theme.accent, 0.16) : Qt.alpha(Theme.accent, 0.06)
        border.color: drop.containsDrag ? Theme.accent : Theme.border
        DropArea {
            id: drop
            anchors.fill: parent
            enabled: root.ready && !root.busy
            onEntered: drag => {
                drag.accepted = drag.hasUrls && !!(drag.supportedActions & Qt.CopyAction);
            }
            onDropped: event => root.handleDrop(event)
        }
        ColumnLayout {
            id: addBox
            x: 12
            y: 12
            width: parent.width - 24
            spacing: 8
            Text {
                text: "Drop files or folders here"
                color: Theme.foreground
                font.pixelSize: 12
                font.weight: Font.DemiBold
            }
            Text {
                text: "References only. Your originals stay in place. The shelf clears when Omnibar restarts."
                color: Theme.muted
                font.pixelSize: 11
                wrapMode: Text.Wrap
                Layout.fillWidth: true
            }
            RowLayout {
                Layout.fillWidth: true
                TextField {
                    id: pathInput
                    objectName: "shelfPath"
                    Layout.fillWidth: true
                    placeholderText: "/home/you/path or file:///…"
                    Accessible.name: "File or folder path to add"
                    enabled: root.ready && !root.busy
                    color: Theme.foreground
                    placeholderTextColor: Theme.muted
                    selectionColor: Theme.accent
                    selectedTextColor: Theme.background
                    leftPadding: 10
                    rightPadding: 10
                    background: Rectangle {
                        radius: 9
                        color: Theme.surface
                        border.color: pathInput.activeFocus ? Theme.accent : Theme.border
                    }
                    onAccepted: root.addPaths([text])
                }
                PanelButton {
                    objectName: "addShelf"
                    text: "Add"
                    enabled: root.ready && !root.busy && pathInput.text.length > 0
                    onClicked: root.addPaths([pathInput.text])
                }
            }
        }
    }
    RowLayout {
        Layout.fillWidth: true
        TextField {
            id: search
            objectName: "shelfSearch"
            Layout.fillWidth: true
            placeholderText: "Search shelf…"
            Accessible.name: "Search file shelf"
            enabled: root.ready
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
        PanelButton {
            objectName: "clearShelf"
            text: "Clear shelf"
            description: "Remove all shelf references; original files stay in place"
            enabled: root.ready && (root.snapshot.entries || []).length > 0
            onClicked: root.clearRequested()
        }
    }
    Text {
        visible: root.ready && rows.count === 0
        text: search.text ? "No matching files." : "Add a file or folder to keep it within reach."
        color: Theme.muted
        font.pixelSize: 12
        Layout.fillWidth: true
        wrapMode: Text.Wrap
    }
    Repeater {
        model: rows
        delegate: Rectangle {
            required property string entryId
            required property string filePath
            required property string fileName
            required property bool directory
            required property bool available
            required property double fileSize
            objectName: "shelf-" + entryId
            Layout.fillWidth: true
            implicitHeight: card.implicitHeight + 24
            radius: 14
            color: Qt.alpha(Theme.surface, 0.55)
            border.color: Theme.border
            ColumnLayout {
                id: card
                x: 12
                y: 12
                width: parent.width - 24
                spacing: 7
                RowLayout {
                    Layout.fillWidth: true
                    Text {
                        text: directory ? "▤" : "◇"
                        color: Theme.accent
                        font.pixelSize: 18
                    }
                    Text {
                        text: fileName
                        textFormat: Text.PlainText
                        color: Theme.foreground
                        font.pixelSize: 13
                        font.weight: Font.DemiBold
                        Layout.fillWidth: true
                        elide: Text.ElideMiddle
                    }
                    Text {
                        text: !available ? "Unavailable" : directory ? "Folder" : Format.formatBytes(fileSize)
                        color: Theme.muted
                        font.pixelSize: 10
                    }
                }
                Text {
                    text: filePath
                    textFormat: Text.PlainText
                    color: Theme.muted
                    font.pixelSize: 11
                    Layout.fillWidth: true
                    elide: Text.ElideMiddle
                    maximumLineCount: 1
                }
                RowLayout {
                    Layout.fillWidth: true
                    PanelButton {
                        objectName: "shelfOpen"
                        text: "Open folder"
                        enabled: root.ready && available
                        onClicked: root.folderRequested(entryId)
                    }
                    Item {
                        Layout.fillWidth: true
                    }
                    PanelButton {
                        objectName: "shelfCopy"
                        text: "Copy path"
                        description: root.snapshot.can_copy ? "Copy this absolute path" : "Enable clipboard.write to copy paths"
                        enabled: root.ready && available && root.snapshot.can_copy === true
                        onClicked: root.copyRequested(entryId)
                    }
                    PanelButton {
                        objectName: "shelfRemove"
                        text: "Remove"
                        description: "Remove this shelf reference; keep the original file"
                        enabled: root.ready
                        onClicked: root.removeRequested(entryId)
                    }
                }
            }
        }
    }
}
