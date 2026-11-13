import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    property string address: ""
    property int workspaceID: 1
    property bool floating: false
    property int fullscreen: 0
    property bool busy: false
    signal controlRequested(string method, var params)
    spacing: 6
    RowLayout {
        Layout.fillWidth: true
        PanelButton {
            objectName: "toggleFloating"
            text: root.floating ? "Tile" : "Float"
            enabled: !root.busy && root.fullscreen === 0
            onClicked: root.controlRequested("windows.setFloating", {
                address: root.address,
                enabled: !root.floating
            })
        }
        PanelButton {
            objectName: "toggleMaximized"
            text: root.fullscreen === 1 ? "Restore size" : "Maximize"
            enabled: !root.busy && root.fullscreen < 2
            onClicked: root.controlRequested("windows.setMaximized", {
                address: root.address,
                enabled: root.fullscreen !== 1
            })
        }
    }
    RowLayout {
        Layout.fillWidth: true
        Text {
            text: "Workspace ID"
            color: Theme.muted
            font.pixelSize: 11
        }
        SpinBox {
            id: workspace
            objectName: "destinationWorkspace"
            from: 1
            to: 9999
            value: Math.max(1, root.workspaceID)
            editable: true
            enabled: !root.busy && root.fullscreen < 2
            Accessible.name: "Destination Hyprland workspace ID"
        }
        PanelButton {
            objectName: "moveWorkspace"
            text: "Move"
            enabled: !root.busy && root.fullscreen < 2 && workspace.value !== root.workspaceID
            onClicked: root.controlRequested("windows.moveWorkspace", {
                address: root.address,
                workspace: workspace.value
            })
        }
    }
    Text {
        Layout.fillWidth: true
        text: root.fullscreen >= 2 ? "Exit fullscreen to change this window." : "Move keeps you on this workspace. Changes keep this panel open."
        wrapMode: Text.WordWrap
        color: Theme.muted
        font.pixelSize: 10
    }
}
