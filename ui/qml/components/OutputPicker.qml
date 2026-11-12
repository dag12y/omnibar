import QtQuick
import QtQuick.Controls

// Reuse the options model across metric refreshes. A real device-list change
// closes the popup so a selection cannot refer to an obsolete row index.
ComboBox {
    id: root
    property var outputs: []
    property string selectedOutput: ""
    property bool pending: false
    property string optionSignature: ""
    signal outputChosen(string output)
    implicitHeight: 36
    leftPadding: 12
    rightPadding: 32
    font.pixelSize: 12
    textRole: "description"
    valueRole: "name"
    model: options
    displayText: currentIndex >= 0 ? currentText : count === 0 ? "No outputs connected" : "Output unavailable"
    Accessible.name: "Playback output"
    ToolTip.visible: hovered
    ToolTip.delay: 600
    ToolTip.text: Accessible.name + " · " + displayText
    ListModel {
        id: options
    }
    function syncSelection() {
        let index = -1;
        for (let i = 0; i < options.count; i++)
            if (options.get(i).name === selectedOutput) {
                index = i;
                break;
            }
        currentIndex = index;
    }
    function syncOptions() {
        const signature = JSON.stringify(outputs.map(item => [item.id, item.name, item.description]));
        if (signature === optionSignature)
            return;
        optionSignature = signature;
        popup.close();
        options.clear();
        for (const item of outputs)
            options.append({
                name: item.name,
                description: item.description || item.name
            });
        syncSelection();
    }
    onOutputsChanged: syncOptions()
    onSelectedOutputChanged: {
        if (!pending)
            syncSelection();
    }
    onPendingChanged: {
        if (!pending)
            syncSelection();
    }
    Component.onCompleted: syncOptions()
    onActivated: index => {
        if (index >= 0 && index < options.count)
            outputChosen(options.get(index).name);
    }
    palette.text: Theme.foreground
    palette.buttonText: Theme.foreground
    palette.base: Theme.background
    palette.highlight: Theme.accent
    palette.highlightedText: Theme.background
    contentItem: Text {
        text: root.displayText
        textFormat: Text.PlainText
        font: root.font
        color: root.enabled ? Theme.foreground : Theme.muted
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
    }
    background: Rectangle {
        radius: 10
        color: Theme.surface
        border.color: root.visualFocus ? Theme.accent : Theme.border
    }
}
