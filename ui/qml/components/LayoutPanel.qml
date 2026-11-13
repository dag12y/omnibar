import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    property var snapshot: ({
            windows: [],
            layouts: []
        })
    property bool active: visible
    property bool busy: false
    property string selectedID: ""
    signal layoutRequested(string method, var params)
    spacing: 8
    ListModel {
        id: layouts
    }
    function save() {
        if (!busy && !snapshot.layoutError && name.text.trim().length > 0)
            layoutRequested("windows.saveLayout", {
                name: name.text.trim(),
                workspace: workspace.value
            });
    }
    function sync() {
        const wanted = snapshot.layouts || [];
        for (let i = layouts.count - 1; i >= 0; i--)
            if (!wanted.some(l => l.id === layouts.get(i).layoutID))
                layouts.remove(i);
        for (let i = 0; i < wanted.length; i++) {
            let found = -1;
            for (let j = i; j < layouts.count; j++)
                if (layouts.get(j).layoutID === wanted[i].id) {
                    found = j;
                    break;
                }
            const row = {
                layoutID: wanted[i].id,
                layoutName: wanted[i].name + " · " + wanted[i].count + " windows"
            };
            if (found < 0)
                layouts.insert(i, row);
            else {
                if (found !== i)
                    layouts.move(found, i, 1);
                layouts.set(i, row);
            }
        }
        let index = -1;
        for (let i = 0; i < layouts.count; i++)
            if (layouts.get(i).layoutID === selectedID)
                index = i;
        if (index < 0 && layouts.count) {
            index = 0;
            selectedID = layouts.get(0).layoutID;
        }
        if (!layouts.count)
            selectedID = "";
        picker.currentIndex = index;
    }
    function initializeWorkspace() {
        const windows = snapshot.windows || [];
        const current = windows.find(w => w.active) || windows[0];
        workspace.value = current ? Math.max(1, Math.min(9999, current.workspaceID || 1)) : 1;
    }
    onSnapshotChanged: sync()
    onActiveChanged: {
        if (active)
            initializeWorkspace();
        else {
            name.text = "";
            selectedID = "";
        }
    }
    Component.onCompleted: {
        sync();
        if (active)
            initializeWorkspace();
    }
    Text {
        Layout.fillWidth: true
        text: "Save workspace placement and floating-window positions. Save updates an existing name."
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 11
    }
    TextField {
        id: name
        objectName: "layoutName"
        Layout.fillWidth: true
        maximumLength: 64
        placeholderText: "Layout name…"
        onAccepted: root.save()
        Accessible.name: "Layout name"
        enabled: !root.busy && !root.snapshot.layoutError
        color: Theme.foreground
        background: Rectangle {
            color: Theme.surface
            radius: 8
            border.color: name.activeFocus ? Theme.accent : Theme.border
        }
    }
    RowLayout {
        Layout.fillWidth: true
        Text {
            text: "Workspace ID"
            font.pixelSize: 11
            color: Theme.muted
        }
        SpinBox {
            id: workspace
            objectName: "layoutWorkspace"
            from: 1
            to: 9999
            editable: true
            enabled: !root.busy
            Accessible.name: "Workspace ID to save"
        }
        PanelButton {
            objectName: "saveLayout"
            text: "Save"
            enabled: !root.busy && name.text.trim().length > 0 && !root.snapshot.layoutError
            onClicked: root.layoutRequested("windows.saveLayout", {
                name: name.text.trim(),
                workspace: workspace.value
            })
        }
    }
    ComboBox {
        id: picker
        objectName: "savedLayouts"
        Layout.fillWidth: true
        model: layouts
        textRole: "layoutName"
        valueRole: "layoutID"
        enabled: !root.busy && layouts.count > 0
        Accessible.name: "Saved layout"
        onActivated: root.selectedID = layouts.get(currentIndex).layoutID
    }
    RowLayout {
        PanelButton {
            objectName: "restoreLayout"
            text: "Restore"
            enabled: !root.busy && root.selectedID !== "" && !root.snapshot.layoutError
            onClicked: root.layoutRequested("windows.restoreLayout", {
                id: root.selectedID
            })
        }
        PanelButton {
            objectName: "deleteLayout"
            text: "Delete layout"
            enabled: !root.busy && root.selectedID !== "" && !root.snapshot.layoutError
            onClicked: root.layoutRequested("windows.deleteLayout", {
                id: root.selectedID
            })
        }
    }
    Text {
        Layout.fillWidth: true
        text: root.snapshot.layoutError || "Open the matching apps before restoring. Tiled windows follow Hyprland's current layout. Saved layouts include app names and window titles."
        wrapMode: Text.WordWrap
        textFormat: Text.PlainText
        font.pixelSize: 10
        color: Theme.muted
    }
}
