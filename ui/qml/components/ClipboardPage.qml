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
    property bool favoritesOnly: false
    property string transformEntryId: ""
    readonly property int favoriteCount: (snapshot.entries || []).filter(entry => entry.favorite === true).length
    readonly property bool ready: state === "ready"
    signal collectionRequested(bool collecting)
    signal copyRequested(string entryId)
    signal transformRequested(string entryId, string operation)
    signal favoriteRequested(string entryId, bool favorite)
    signal deleteRequested(string entryId)
    signal clearRequested
    signal refreshRequested
    spacing: 14
    ListModel {
        id: entries
    }
    function syncRows() {
        if (!(snapshot.entries || []).some(entry => entry.id === transformEntryId))
            transformEntryId = "";
        const query = search.text.toLowerCase();
        const wanted = (snapshot.entries || []).filter(entry => (!favoritesOnly || entry.favorite === true) && entry.text.toLowerCase().includes(query));
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
                copiedAt: entry.copied_at,
                favorite: entry.favorite === true
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
    onFavoritesOnlyChanged: syncRows()
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
                text: "Recent copies and saved favorites"
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
        visible: !root.ready || root.errorText.length > 0 || !!root.snapshot.error || !!root.snapshot.favorites_error
        text: root.state === "denied" ? "Enable clipboard.read in your Omnibar config and restart the backend." : root.state === "disabled" ? "Clipboard history is disabled in your config." : root.state === "offline" ? "Backend unavailable. Reconnecting…" : root.errorText || root.snapshot.error || root.snapshot.favorites_error || "Loading clipboard history…"
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
                text: "Recent copies stay for this session. Save favorite writes an item to disk for next time. Collection continues while hidden."
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
                    text: "Clear recent"
                    enabled: root.ready && (root.snapshot.entries || []).some(entry => entry.favorite !== true)
                    onClicked: root.clearRequested()
                }
            }
        }
    }
    RowLayout {
        Layout.fillWidth: true
        PanelButton {
            objectName: "allHistory"
            text: "All copies"
            selected: !root.favoritesOnly
            onClicked: root.favoritesOnly = false
        }
        PanelButton {
            objectName: "favoritesFilter"
            text: "Favorites · " + root.favoriteCount
            selected: root.favoritesOnly
            onClicked: root.favoritesOnly = true
        }
        Item {
            Layout.fillWidth: true
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
        text: search.text ? "No matching copied text." : root.favoritesOnly ? "Save a favorite to keep it here across restarts." : "Copied text will appear here after you start history."
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
            required property bool favorite
            objectName: "history-" + entryId
            Layout.fillWidth: true
            implicitHeight: item.implicitHeight + 24
            radius: 14
            color: Qt.alpha(Theme.surface, 0.55)
            border.color: favorite ? Qt.alpha(Theme.accent, 0.55) : Theme.border
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
                        text: favorite ? "★ SAVED" : copiedAt ? Qt.formatTime(new Date(copiedAt), "hh:mm") : ""
                        color: Theme.muted
                        font.pixelSize: 10
                        Layout.fillWidth: true
                    }
                    PanelButton {
                        objectName: "favoriteHistory"
                        text: favorite ? "Unsave" : "Save favorite"
                        description: favorite ? "Stop saving this item across restarts" : "Save this text to disk as a favorite"
                        enabled: root.ready && root.snapshot.can_favorite === true
                        onClicked: root.favoriteRequested(entryId, !favorite)
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
                        text: favorite ? "Remove" : "Delete"
                        description: favorite ? "Remove this saved favorite from disk and history" : "Remove this history item"
                        enabled: root.ready
                        onClicked: root.deleteRequested(entryId)
                    }
                }
                PanelButton {
                    objectName: "transformHistory"
                    text: root.transformEntryId === entryId ? "Close transforms" : "Transform & copy…"
                    description: root.snapshot.can_copy ? "Copy a transformed version without editing this item" : "Enable clipboard.write to transform and copy"
                    selected: root.transformEntryId === entryId
                    enabled: root.ready && root.snapshot.can_copy === true && !root.busy
                    onClicked: root.transformEntryId = root.transformEntryId === entryId ? "" : entryId
                }
                ColumnLayout {
                    visible: root.transformEntryId === entryId
                    Layout.fillWidth: true
                    spacing: 6
                    Text {
                        text: "Copy a new version. This item stays unchanged."
                        color: Theme.muted
                        font.pixelSize: 11
                        wrapMode: Text.WordWrap
                        Layout.fillWidth: true
                    }
                    GridLayout {
                        columns: 2
                        Layout.fillWidth: true
                        columnSpacing: 8
                        rowSpacing: 4
                        Repeater {
                            model: [
                                {
                                    operation: "trim",
                                    label: "Trim edges",
                                    detail: "Remove whitespace at the start and end"
                                },
                                {
                                    operation: "lowercase",
                                    label: "lowercase",
                                    detail: "Copy in lowercase"
                                },
                                {
                                    operation: "uppercase",
                                    label: "UPPERCASE",
                                    detail: "Copy in uppercase"
                                },
                                {
                                    operation: "remove_line_breaks",
                                    label: "Remove line breaks",
                                    detail: "Replace each line break with a space"
                                }
                            ]
                            PanelButton {
                                required property var modelData
                                objectName: "transform-" + modelData.operation
                                Layout.fillWidth: true
                                text: modelData.label
                                description: modelData.detail
                                enabled: root.ready && root.snapshot.can_copy === true && !root.busy
                                onClicked: root.transformRequested(entryId, modelData.operation)
                            }
                        }
                    }
                }
            }
        }
    }
}
