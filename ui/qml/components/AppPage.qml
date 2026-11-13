import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    property var snapshot: ({
            apps: [],
            truncated: false
        })
    property string state: "pending"
    property string errorText: ""
    property bool busy: false
    readonly property bool ready: state === "ready"
    signal launchRequested(string appID)
    signal refreshRequested
    spacing: 14
    ListModel {
        id: rows
    }
    function syncRows() {
        const query = search.text.toLowerCase();
        const wanted = (snapshot.apps || []).filter(app => (app.name + " " + app.description + " " + app.keywords).toLowerCase().includes(query));
        for (let i = rows.count - 1; i >= 0; i--)
            if (!wanted.some(app => app.id === rows.get(i).appID))
                rows.remove(i);
        for (let i = 0; i < wanted.length; i++) {
            const app = wanted[i];
            let current = -1;
            for (let j = i; j < rows.count; j++)
                if (rows.get(j).appID === app.id) {
                    current = j;
                    break;
                }
            const row = {
                appID: app.id,
                appName: app.name,
                appDescription: app.description,
                keywords: app.keywords || ""
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
    onVisibleChanged: if (visible) Qt.callLater(() => search.forceActiveFocus())
    RowLayout {
        Layout.fillWidth: true
        ColumnLayout {
            Layout.fillWidth: true
            Text {
                text: "Apps"
                color: Theme.foreground
                font.pixelSize: 17
                font.weight: Font.DemiBold
            }
            Text {
                text: "Find and open an installed application"
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
        text: root.state === "denied" ? "Enable process.manage in ~/.config/omnibar/config.toml and restart the backend." : root.state === "disabled" ? "Application launcher is disabled in your config." : root.state === "offline" ? "Backend unavailable. Reconnecting…" : root.errorText || "Loading installed apps…"
        color: Theme.muted
        font.pixelSize: 12
        wrapMode: Text.WordWrap
        textFormat: Text.PlainText
    }
    TextField {
        id: search
        objectName: "appSearch"
        Layout.fillWidth: true
        implicitHeight: 38
        placeholderText: "Search apps…"
        Accessible.name: "Search installed apps"
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
            root.launchRequested(rows.get(0).appID)
    }
    Text {
        text: "Launch opens an installed app and closes this panel when the request is accepted."
        Layout.fillWidth: true
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 11
    }
    Text {
        visible: root.ready && root.snapshot.truncated === true
        text: "Showing up to 200 apps. Some installed apps may be omitted."
        Layout.fillWidth: true
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 11
    }
    Text {
        visible: root.ready && rows.count === 0
        text: search.text ? "No matching apps." : "No installed applications found."
        color: Theme.muted
        font.pixelSize: 12
        Layout.fillWidth: true
        wrapMode: Text.WordWrap
    }
    Repeater {
        model: rows
        delegate: Rectangle {
            required property string appID
            required property string appName
            required property string appDescription
            required property string keywords
            objectName: "app-" + appID
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
                spacing: 6
                RowLayout {
                    Layout.fillWidth: true
                    Text {
                        text: appName
                        textFormat: Text.PlainText
                        color: Theme.foreground
                        font.pixelSize: 14
                        font.weight: Font.DemiBold
                        Layout.fillWidth: true
                        elide: Text.ElideRight
                    }
                }
                Text {
                    text: appDescription
                    visible: appDescription.length > 0
                    textFormat: Text.PlainText
                    color: Theme.muted
                    font.pixelSize: 11
                    wrapMode: Text.WrapAnywhere
                    maximumLineCount: 2
                    elide: Text.ElideRight
                    Layout.fillWidth: true
                }
                RowLayout {
                    Layout.fillWidth: true
                    Item { Layout.fillWidth: true }
                    PanelButton {
                        objectName: "launchApp"
                        text: "Launch →"
                        description: "Launch " + appName
                        enabled: root.ready && !root.busy
                        onClicked: root.launchRequested(appID)
                    }
                }
            }
        }
    }
}
