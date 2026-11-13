import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    property var snapshot: ({
            windows: [],
            truncated: false
        })
    property string state: "pending"
    property string errorText: ""
    property bool active: visible
    property bool busy: false
    property var preview: null
    signal previewRevealed(var item)
    readonly property bool ready: state === "ready"
    signal focusRequested(string address)
    signal controlRequested(string method, var params)
    property string controlsAddress: ""
    signal refreshRequested
    spacing: 14
    ListModel {
        id: rows
    }
    function syncRows() {
        const query = search.text.toLowerCase();
        const wanted = (snapshot.windows || []).filter(window => (window.title + " " + window.class + " " + window.workspace).toLowerCase().includes(query));
        if (!wanted.some(window => window.address === controlsAddress))
            controlsAddress = "";
        for (let i = rows.count - 1; i >= 0; i--)
            if (!wanted.some(window => window.address === rows.get(i).address))
                rows.remove(i);
        for (let i = 0; i < wanted.length; i++) {
            const window = wanted[i];
            let current = -1;
            for (let j = i; j < rows.count; j++)
                if (rows.get(j).address === window.address) {
                    current = j;
                    break;
                }
            const row = {
                address: window.address,
                windowTitle: window.title,
                appClass: window.class,
                workspace: window.workspace,
                monitor: window.monitor,
                active: window.active === true,
                workspaceID: window.workspaceID || 0,
                floating: window.floating === true,
                fullscreen: window.fullscreen || 0,
                restricted: window.restricted === true
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
    function focusSearch() {
        if (active && ready)
            search.forceActiveFocus();
    }
    onActiveChanged: {
        if (active)
            Qt.callLater(focusSearch);
        else
            controlsAddress = "";
    }
    onReadyChanged: if (active && ready)
        Qt.callLater(focusSearch)
    Component.onCompleted: syncRows()
    RowLayout {
        Layout.fillWidth: true
        ColumnLayout {
            Layout.fillWidth: true
            Text {
                text: "Windows"
                color: Theme.foreground
                font.pixelSize: 17
                font.weight: Font.DemiBold
            }
            Text {
                text: "Find your place and switch back"
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
        text: root.state === "denied" ? "Enable window.manage in ~/.config/omnibar/config.toml and restart the backend." : root.state === "disabled" ? "Window switcher is disabled in your config." : root.state === "offline" ? "Backend unavailable. Reconnecting…" : root.errorText || "Loading running windows…"
        color: Theme.muted
        font.pixelSize: 12
        wrapMode: Text.WordWrap
        textFormat: Text.PlainText
    }
    Text {
        Layout.fillWidth: true
        visible: root.ready && root.snapshot.previewsAllowed !== true
        text: "Optional previews require screen.capture in your config."
        color: Theme.muted
        font.pixelSize: 11
        wrapMode: Text.WordWrap
    }
    TextField {
        id: search
        objectName: "windowSearch"
        Layout.fillWidth: true
        implicitHeight: 38
        placeholderText: "Search app, title, or workspace…"
        Accessible.name: "Search running windows"
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
        onTextChanged: root.syncRows()
        onAccepted: if (root.ready && !root.busy && rows.count > 0)
            root.focusRequested(rows.get(0).address)
    }
    Text {
        text: "Switch focuses an existing window and closes this panel."
        Layout.fillWidth: true
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 11
    }
    Text {
        visible: root.ready && root.snapshot.truncated === true
        text: "Showing the first 100 windows. Close unused windows to see more."
        Layout.fillWidth: true
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 11
    }
    Text {
        visible: root.ready && rows.count === 0
        text: search.text ? "No matching windows." : "No switchable windows are open."
        color: Theme.muted
        font.pixelSize: 12
        Layout.fillWidth: true
        wrapMode: Text.WordWrap
    }
    Repeater {
        model: rows
        delegate: Rectangle {
            required property string address
            required property string windowTitle
            required property string appClass
            required property string workspace
            required property int monitor
            required property bool active
            required property int workspaceID
            required property bool floating
            required property int fullscreen
            required property bool restricted
            objectName: "window-" + address
            Layout.fillWidth: true
            implicitHeight: card.implicitHeight + 24
            radius: 14
            color: Qt.alpha(Theme.surface, 0.55)
            border.color: active ? Qt.alpha(Theme.accent, 0.55) : Theme.border
            ColumnLayout {
                id: card
                x: 12
                y: 12
                width: parent.width - 24
                spacing: 6
                RowLayout {
                    Layout.fillWidth: true
                    Text {
                        text: appClass
                        textFormat: Text.PlainText
                        color: Theme.accent
                        font.pixelSize: 12
                        font.weight: Font.DemiBold
                        Layout.fillWidth: true
                        elide: Text.ElideRight
                    }
                    Text {
                        text: active ? "● ACTIVE" : ""
                        color: Theme.muted
                        font.pixelSize: 10
                    }
                }
                Text {
                    text: windowTitle
                    textFormat: Text.PlainText
                    color: Theme.foreground
                    font.pixelSize: 13
                    wrapMode: Text.WrapAnywhere
                    maximumLineCount: 2
                    elide: Text.ElideRight
                    Layout.fillWidth: true
                }
                WindowThumbnail {
                    controller: root.preview
                    address: card.parent.address
                    Layout.fillWidth: true
                    onRevealRequested: item => root.previewRevealed(item)
                }
                RowLayout {
                    Layout.fillWidth: true
                    Text {
                        text: "Workspace " + (workspace || "?") + " · Monitor " + monitor
                        textFormat: Text.PlainText
                        color: Theme.muted
                        font.pixelSize: 10
                        Layout.fillWidth: true
                        elide: Text.ElideRight
                    }
                    PanelButton {
                        objectName: "previewWindow"
                        text: "Preview"
                        description: "Preview " + appClass
                        enabled: root.preview !== null && root.snapshot.previewsAllowed === true && root.ready && !root.busy && !root.preview.busy
                        onClicked: root.preview.request(address)
                    }
                    PanelButton {
                        objectName: "showControls"
                        text: "Controls"
                        visible: root.snapshot.controlsAvailable === true
                        selected: root.controlsAddress === address
                        enabled: root.ready && !root.busy && !restricted
                        onClicked: root.controlsAddress = root.controlsAddress === address ? "" : address
                    }
                    PanelButton {
                        objectName: "switchWindow"
                        text: "Switch →"
                        description: "Switch to " + appClass
                        enabled: root.ready && !root.busy
                        onClicked: root.focusRequested(address)
                    }
                }
                WindowControls {
                    visible: root.controlsAddress === card.parent.address
                    Layout.fillWidth: true
                    address: card.parent.address
                    workspaceID: card.parent.workspaceID
                    floating: card.parent.floating
                    fullscreen: card.parent.fullscreen
                    busy: root.busy || card.parent.restricted
                    onControlRequested: (method, params) => root.controlRequested(method, params)
                }
            }
        }
    }
}
