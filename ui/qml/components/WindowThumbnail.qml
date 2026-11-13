import QtQuick
import QtQuick.Layouts

ColumnLayout {
    id: root
    property var controller: null
    property string address: ""
    readonly property bool selected: controller !== null && controller.active && controller.address === address && address !== ""
    signal revealRequested(var item)
    visible: selected
    spacing: 4
    RowLayout {
        Layout.fillWidth: true
        Text {
            text: "Window preview · snapshot"
            color: Theme.muted
            font.pixelSize: 10
            Layout.fillWidth: true
        }
        PanelButton {
            text: "×"
            description: "Hide window preview"
            onClicked: if (root.controller)
                root.controller.clear()
        }
    }
    Image {
        id: image
        objectName: "windowThumbnail"
        Layout.fillWidth: true
        Layout.preferredHeight: source.toString().length > 0 ? 160 : 0
        source: root.selected ? root.controller.imageSource : ""
        cache: false
        sourceSize: Qt.size(320, 200)
        fillMode: Image.PreserveAspectFit
        onStatusChanged: if (status === Image.Ready)
            Qt.callLater(() => {
                if (root.selected && image.status === Image.Ready) {
                    console.info("[omnibar-ui] window preview ready");
                    root.revealRequested(root);
                }
            })
    }
    Text {
        Layout.fillWidth: true
        visible: root.selected && (root.controller.busy || root.controller.errorText.length > 0)
        text: root.selected ? (root.controller.busy ? "Capturing preview…" : root.controller.errorText) : ""
        wrapMode: Text.WordWrap
        textFormat: Text.PlainText
        color: Theme.muted
        font.pixelSize: 11
    }
}
