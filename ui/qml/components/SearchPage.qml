import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import "search.js" as Search

ColumnLayout {
    id: root
    property bool active: visible
    property var windowSnapshot: ({
            windows: []
        })
    property var appSnapshot: ({
            apps: []
        })
    property string windowState: "pending"
    property string appState: "pending"
    property string windowError: ""
    property string appError: ""
    property bool busy: false
    property var preview: null
    readonly property bool ready: windowState === "ready" || appState === "ready"
    property string selectedKey: ""
    property bool selectionExplicit: false
    property bool syncing: false
    property bool pendingFocus: true
    property int totalResults: 0
    readonly property int resultCount: rows.count
    signal focusRequested(string address)
    signal launchRequested(string appID)
    signal refreshRequested
    signal highlightRequested(var item)
    spacing: 12

    ListModel {
        id: rows
    }
    function syncRows() {
        syncing = true;
        if (!active) {
            rows.clear();
            selectedKey = "";
            totalResults = 0;
            selectionExplicit = false;
            syncing = false;
            return;
        }
        const all = Search.results(windowState === "ready" ? windowSnapshot.windows : [], appState === "ready" ? appSnapshot.apps : [], search.text);
        totalResults = all.length;
        const wanted = all.slice(0, 30);
        for (let i = rows.count - 1; i >= 0; i--)
            if (!wanted.some(row => row.key === rows.get(i).key))
                rows.remove(i);
        for (let i = 0; i < wanted.length; i++) {
            let current = -1;
            for (let j = i; j < rows.count; j++)
                if (rows.get(j).key === wanted[i].key) {
                    current = j;
                    break;
                }
            if (current < 0)
                rows.insert(i, wanted[i]);
            else {
                if (current !== i)
                    rows.move(current, i, 1);
                rows.set(i, wanted[i]);
            }
        }
        if (!selectionExplicit || !wanted.some(row => row.key === selectedKey)) {
            selectedKey = rows.count ? rows.get(0).key : "";
            selectionExplicit = false;
        }
        syncing = false;
    }
    function moveSelection(delta) {
        if (!rows.count)
            return;
        let index = 0;
        for (let i = 0; i < rows.count; i++)
            if (rows.get(i).key === selectedKey)
                index = i;
        index = Math.max(0, Math.min(rows.count - 1, index + delta));
        selectionExplicit = true;
        selectedKey = rows.get(index).key;
        Qt.callLater(revealSelection);
    }
    function activate(key) {
        if (busy || !active)
            return;
        for (let i = 0; i < rows.count; i++) {
            const row = rows.get(i);
            if (row.key !== key)
                continue;
            if (row.kind === "window" && windowState === "ready")
                focusRequested(row.target);
            if (row.kind === "app" && appState === "ready")
                launchRequested(row.target);
            return;
        }
    }
    function focusQuery() {
        pendingFocus = true;
        focusSearch();
        search.selectAll();
    }
    function focusSearch() {
        if (active && ready && pendingFocus) {
            search.forceActiveFocus();
            pendingFocus = false;
            revealSearch();
        }
    }
    function revealSearch() {
        if (active)
            highlightRequested(search);
    }
    function revealSelection() {
        if (!active)
            return;
        for (let i = 0; i < cards.count; i++) {
            const card = cards.itemAt(i);
            if (card && card.rowKey === selectedKey) {
                highlightRequested(card);
                return;
            }
        }
    }
    onWindowSnapshotChanged: syncRows()
    onAppSnapshotChanged: syncRows()
    onWindowStateChanged: syncRows()
    onAppStateChanged: syncRows()
    onActiveChanged: {
        selectionExplicit = false;
        pendingFocus = active;
        if (!active)
            search.text = "";
        syncRows();
        if (active) {
            Qt.callLater(focusSearch);
            Qt.callLater(revealSearch);
        }
    }
    onReadyChanged: {
        if (!ready && active)
            pendingFocus = true;
        if (ready && active)
            Qt.callLater(focusSearch);
    }
    Component.onCompleted: {
        syncRows();
        Qt.callLater(focusSearch);
    }

    RowLayout {
        Layout.fillWidth: true
        Text {
            text: "Search"
            color: Theme.foreground
            font.pixelSize: 17
            font.weight: Font.DemiBold
            Layout.fillWidth: true
        }
        PanelButton {
            text: "Refresh ↻"
            enabled: !root.busy && (root.windowState !== "offline" || root.appState !== "offline")
            onClicked: root.refreshRequested()
        }
    }
    TextField {
        id: search
        objectName: "unifiedSearch"
        Layout.fillWidth: true
        implicitHeight: 38
        placeholderText: "Find an app or window…"
        Accessible.name: "Search apps and windows"
        enabled: root.ready
        color: Theme.foreground
        placeholderTextColor: Theme.muted
        selectionColor: Theme.accent
        selectedTextColor: Theme.background
        leftPadding: 12
        rightPadding: 12
        background: Rectangle {
            radius: 10
            color: Theme.surface
            border.color: search.activeFocus ? Theme.accent : Theme.border
        }
        onTextChanged: {
            root.selectionExplicit = false;
            root.syncRows();
            Qt.callLater(root.revealSearch);
        }
        onAccepted: root.activate(root.selectedKey)
        Keys.onDownPressed: root.moveSelection(1)
        Keys.onUpPressed: root.moveSelection(-1)
    }
    Text {
        Layout.fillWidth: true
        text: "Switch to a running window or launch an app. ↑ ↓ to select, Enter to open."
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 11
    }
    Text {
        Layout.fillWidth: true
        visible: root.windowState === "ready" && root.windowSnapshot.previewsAllowed !== true
        text: "Optional previews require screen.capture in your config."
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 11
    }
    Repeater {
        model: [
            {
                label: "Windows",
                sourceState: root.windowState,
                errorText: root.windowError,
                grant: "window.manage"
            },
            {
                label: "Apps",
                sourceState: root.appState,
                errorText: root.appError,
                grant: "process.manage"
            }
        ]
        delegate: Text {
            required property var modelData
            Layout.fillWidth: true
            visible: modelData.sourceState !== "ready" || modelData.errorText.length > 0
            text: modelData.label + ": " + (modelData.sourceState === "denied" ? "Enable " + modelData.grant + " in your config and restart the backend." : modelData.sourceState === "disabled" ? "Disabled in your config." : modelData.sourceState === "offline" ? "Backend unavailable. Reconnecting…" : modelData.errorText || "Loading…")
            wrapMode: Text.WordWrap
            textFormat: Text.PlainText
            color: Theme.muted
            font.pixelSize: 11
        }
    }
    Text {
        Layout.fillWidth: true
        visible: root.totalResults > 30 || root.windowSnapshot.truncated === true || root.appSnapshot.truncated === true
        text: "Showing a limited set of results. Keep typing to narrow your search."
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 11
    }
    Text {
        visible: root.ready && rows.count === 0
        text: search.text ? "No matching apps or windows." : "No available apps or windows."
        Layout.fillWidth: true
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 12
    }
    Repeater {
        id: cards
        model: rows
        delegate: Rectangle {
            id: card
            required property string key
            required property string target
            required property string kind
            required property string title
            required property string detail
            required property bool active
            readonly property string rowKey: key
            readonly property bool selected: root.selectedKey === key
            objectName: "search-" + key
            Layout.fillWidth: true
            implicitHeight: content.implicitHeight + 20
            radius: 12
            color: Qt.alpha(Theme.surface, 0.55)
            border.color: selected ? Theme.accent : Theme.border
            ColumnLayout {
                id: content
                x: 10
                y: 10
                width: parent.width - 20
                spacing: 5
                Text {
                    Layout.fillWidth: true
                    text: card.title
                    textFormat: Text.PlainText
                    color: Theme.foreground
                    font.pixelSize: 13
                    font.weight: Font.DemiBold
                    maximumLineCount: 2
                    wrapMode: Text.WrapAnywhere
                    elide: Text.ElideRight
                }
                Text {
                    Layout.fillWidth: true
                    text: card.detail
                    textFormat: Text.PlainText
                    color: Theme.muted
                    font.pixelSize: 11
                    elide: Text.ElideRight
                }
                WindowThumbnail {
                    controller: root.preview
                    address: card.kind === "window" ? card.target : ""
                    Layout.fillWidth: true
                    onRevealRequested: item => root.highlightRequested(item)
                }
                RowLayout {
                    Layout.fillWidth: true
                    Text {
                        text: card.kind === "window" ? (card.active ? "WINDOW · ACTIVE" : "WINDOW") : "APP"
                        color: Theme.muted
                        font.pixelSize: 10
                        Layout.fillWidth: true
                    }
                    PanelButton {
                        objectName: "searchPreview"
                        visible: card.kind === "window"
                        text: "Preview"
                        description: "Preview " + card.title
                        enabled: root.preview !== null && root.windowSnapshot.previewsAllowed === true && !root.busy && !root.preview.busy && root.windowState === "ready"
                        onActiveFocusChanged: if (activeFocus && !root.syncing) {
                            root.selectionExplicit = true;
                            root.selectedKey = card.key;
                        }
                        onClicked: root.preview.request(card.target)
                    }
                    PanelButton {
                        objectName: "searchAction"
                        text: card.kind === "window" ? "Switch →" : "Launch →"
                        description: (card.kind === "window" ? "Switch to " : "Launch ") + card.title
                        enabled: !root.busy && (card.kind === "window" ? root.windowState === "ready" : root.appState === "ready")
                        onActiveFocusChanged: if (activeFocus && !root.syncing) {
                            root.selectionExplicit = true;
                            root.selectedKey = card.key;
                        }
                        onClicked: root.activate(card.key)
                    }
                }
            }
        }
    }
}
