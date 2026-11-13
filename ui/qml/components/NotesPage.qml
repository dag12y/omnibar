import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    required property var controller
    signal revealRequested(var item)
    spacing: 12
    function focusStart() {
        if (visible && controller.active && controller.state === "ready") {
            if (!controller.noteID)
                newButton.forceActiveFocus();
        }
    }
    onVisibleChanged: if (visible)
        Qt.callLater(focusStart)
    Shortcut {
        sequence: "Ctrl+F"
        enabled: root.visible && root.controller.active
        onActivated: search.forceActiveFocus()
    }
    RowLayout {
        Layout.fillWidth: true
        Text {
            text: "Notes"
            color: Theme.foreground
            font.pixelSize: 17
            font.weight: Font.DemiBold
            Layout.fillWidth: true
        }
        PanelButton {
            id: newButton
            objectName: "newNote"
            text: "New note"
            enabled: root.controller.connected && root.controller.state === "ready" && !root.controller.dirty && !root.controller.busy && !root.controller.snapshot.storageError
            onClicked: root.controller.create()
        }
    }
    Text {
        Layout.fillWidth: true
        text: root.controller.snapshot.storageError || (root.controller.state === "denied" ? "Enable notes.manage in your Omnibar config and restart the backend." : root.controller.state === "disabled" ? "Notes is disabled in your config." : root.controller.errorText || "Plain-text notes, saved locally. Copy requires clipboard permission.")
        color: root.controller.errorText || root.controller.snapshot.storageError ? Theme.accent : Theme.muted
        textFormat: Text.PlainText
        wrapMode: Text.WordWrap
        font.pixelSize: 11
    }
    TextField {
        id: search
        objectName: "noteSearch"
        Layout.fillWidth: true
        placeholderText: "Search note titles and text…"
        maximumLength: 128
        color: Theme.foreground
        enabled: root.controller.state === "ready"
        onTextEdited: root.controller.query = text
        Accessible.name: "Search notes"
        background: Rectangle {
            radius: 8
            color: Theme.surface
            border.color: search.activeFocus ? Theme.accent : Theme.border
        }
    }
    Text {
        visible: root.controller.snapshot.notes.length === 0
        text: root.controller.query ? "No matching notes." : "Create a note to get started."
        color: Theme.muted
        font.pixelSize: 12
    }
    ComboBox {
        id: picker
        objectName: "notePicker"
        Layout.fillWidth: true
        model: root.controller.snapshot.notes.map(n => ({
                    label: (n.pinned ? "★ " : "") + n.title,
                    noteID: n.id
                }))
        textRole: "label"
        enabled: !root.controller.dirty && !root.controller.busy && root.controller.connected
        visible: count > 0
        currentIndex: {
            const notes = root.controller.snapshot.notes;
            return notes.findIndex(n => n.id === root.controller.noteID);
        }
        displayText: currentIndex >= 0 ? currentText : "Choose a note…"
        Accessible.name: "Saved notes"
        onActivated: index => root.controller.open(model[index].noteID)
    }
    ColumnLayout {
        id: editor
        visible: root.controller.noteID !== ""
        Layout.fillWidth: true
        spacing: 10
        TextField {
            id: title
            objectName: "noteTitle"
            Layout.fillWidth: true
            text: root.controller.title
            maximumLength: 128
            enabled: !(root.controller.busy && (root.controller.pendingMethod === "notes.create" || root.controller.pendingMethod === "notes.delete"))
            color: Theme.foreground
            placeholderText: "Note title"
            Accessible.name: "Note title"
            onTextEdited: root.controller.editTitle(text)
            background: Rectangle {
                radius: 8
                color: Theme.surface
                border.color: title.activeFocus ? Theme.accent : Theme.border
            }
        }
        ScrollView {
            id: textScroll
            Layout.fillWidth: true
            Layout.preferredHeight: 200
            clip: true
            ScrollBar.horizontal.policy: ScrollBar.AlwaysOff
            TextArea {
                id: body
                objectName: "noteBody"
                width: textScroll.availableWidth
                enabled: !(root.controller.busy && (root.controller.pendingMethod === "notes.create" || root.controller.pendingMethod === "notes.delete"))
                text: root.controller.body
                textFormat: TextEdit.PlainText
                wrapMode: TextEdit.Wrap
                color: Theme.foreground
                placeholderText: "Write something…"
                selectByMouse: true
                KeyNavigation.priority: KeyNavigation.BeforeItem
                KeyNavigation.tab: pinButton
                KeyNavigation.backtab: title
                Accessible.name: "Note text"
                onTextChanged: if (text !== root.controller.body)
                    root.controller.editBody(text)
                onActiveFocusChanged: if (activeFocus)
                    Qt.callLater(() => root.revealRequested(textScroll))
                background: Rectangle {
                    radius: 8
                    color: Theme.surface
                    border.color: body.activeFocus ? Theme.accent : Theme.border
                }
            }
        }
        RowLayout {
            id: editorActions
            Layout.fillWidth: true
            Text {
                objectName: "noteSaveStatus"
                text: root.controller.saveStatus
                color: root.controller.dirty ? Theme.accent : Theme.muted
                font.pixelSize: 11
                Layout.fillWidth: true
            }
            PanelButton {
                id: pinButton
                objectName: "pinNote"
                enabled: !(root.controller.busy && (root.controller.pendingMethod === "notes.create" || root.controller.pendingMethod === "notes.delete"))
                onActiveFocusChanged: if (activeFocus)
                    root.revealRequested(editorActions)
                text: root.controller.pinned ? "Unpin" : "Pin"
                onClicked: root.controller.togglePin()
            }
            PanelButton {
                objectName: "copyNote"
                onActiveFocusChanged: if (activeFocus)
                    root.revealRequested(editorActions)
                text: "Copy"
                enabled: root.controller.snapshot.canCopy && root.controller.connected && !root.controller.dirty && !root.controller.busy && root.controller.body.length > 0
                onClicked: root.controller.copy()
            }
            PanelButton {
                objectName: "deleteNote"
                onActiveFocusChanged: if (activeFocus)
                    root.revealRequested(editorActions)
                text: "Delete"
                enabled: root.controller.connected && !root.controller.dirty && !root.controller.busy
                onClicked: deleteDialog.open()
            }
        }
        RowLayout {
            visible: root.controller.errorText.length > 0 || root.controller.dirty
            PanelButton {
                text: "Retry"
                enabled: root.controller.connected && !root.controller.busy
                onClicked: root.controller.retry()
            }
            PanelButton {
                text: "Save as new"
                enabled: root.controller.dirty && root.controller.connected && !root.controller.busy
                onClicked: root.controller.saveAsNew()
            }
            PanelButton {
                text: "Reload saved"
                enabled: root.controller.noteID !== "" && root.controller.connected && !root.controller.busy
                onClicked: reloadDialog.open()
            }
        }
        Text {
            visible: root.controller.errorText.length > 0
            Layout.fillWidth: true
            text: root.controller.errorText
            textFormat: Text.PlainText
            color: Theme.accent
            wrapMode: Text.WordWrap
            font.pixelSize: 11
        }
        Text {
            Layout.fillWidth: true
            text: "Autosaves after typing. Unsaved drafts survive hiding this panel, but closing the UI process can lose unsaved changes."
            color: Theme.muted
            font.pixelSize: 10
            wrapMode: Text.WordWrap
        }
    }
    Dialog {
        id: deleteDialog
        objectName: "deleteNoteDialog"
        title: "Delete this note?"
        modal: true
        standardButtons: Dialog.Yes | Dialog.No
        onAccepted: root.controller.remove()
    }
    Dialog {
        id: reloadDialog
        title: "Discard unsaved edits and reload?"
        modal: true
        standardButtons: Dialog.Yes | Dialog.No
        onAccepted: root.controller.reloadSaved()
    }
    Connections {
        target: root.controller
        function onStateChanged() {
            if (root.controller.state === "ready")
                Qt.callLater(root.focusStart);
        }
        function onNoteOpened() {
            if (root.visible)
                Qt.callLater(() => {
                    title.forceActiveFocus();
                    root.revealRequested(editor);
                });
        }
    }
}
