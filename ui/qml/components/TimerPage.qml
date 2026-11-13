import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    required property var controller
    signal revealRequested(var item)
    readonly property bool ready: controller.state === "ready"
    spacing: 12
    function focusName() {
        if (visible && controller.active && ready) {
            name.forceActiveFocus();
            root.revealRequested(name);
        }
    }
    onVisibleChanged: if (visible)
        Qt.callLater(focusName)
    onReadyChanged: if (ready)
        Qt.callLater(focusName)
    RowLayout {
        Layout.fillWidth: true
        Text {
            text: "Timers"
            color: Theme.foreground
            font.pixelSize: 17
            font.weight: Font.DemiBold
            Layout.fillWidth: true
        }
        PanelButton {
            text: "Refresh ↻"
            enabled: !root.controller.busy && root.controller.state !== "offline"
            onClicked: root.controller.request("timers.getSnapshot")
        }
    }
    Text {
        Layout.fillWidth: true
        text: root.controller.state === "denied" ? "Enable timers.manage in your Omnibar config and restart the backend." : root.controller.state === "disabled" ? "Timers is disabled in your config." : root.controller.snapshot.storageError || root.controller.errorText || (!root.ready ? "Loading timers…" : "Timers continue while the panel is hidden. Alerts arrive after waking if a timer finishes during sleep.")
        textFormat: Text.PlainText
        color: Theme.muted
        wrapMode: Text.WordWrap
        font.pixelSize: 11
    }
    TextField {
        id: name
        objectName: "timerName"
        Layout.fillWidth: true
        placeholderText: "Timer name (optional)…"
        placeholderTextColor: Theme.muted
        maximumLength: 64
        color: Theme.foreground
        enabled: root.ready
        Accessible.name: "Timer name"
        background: Rectangle {
            radius: 8
            color: Theme.surface
            border.color: name.activeFocus ? Theme.accent : Theme.border
        }
    }
    RowLayout {
        Layout.fillWidth: true
        Repeater {
            model: [5, 15, 25, 60]
            delegate: PanelButton {
                required property int modelData
                objectName: "preset" + modelData
                Layout.fillWidth: true
                text: modelData + "m"
                description: "Start a " + modelData + " minute timer"
                enabled: root.ready && !root.controller.busy && root.controller.unfinished < 10
                onClicked: root.controller.start(name.text, modelData * 60)
            }
        }
    }
    RowLayout {
        Layout.fillWidth: true
        Text {
            text: "Min"
            color: Theme.muted
            font.pixelSize: 11
        }
        SpinBox {
            id: minutes
            objectName: "timerMinutes"
            from: 0
            to: 1440
            value: 25
            editable: true
            live: true
            Layout.preferredWidth: 88
            enabled: root.ready
            Accessible.name: "Custom timer minutes"
        }
        Text {
            text: "Sec"
            color: Theme.muted
            font.pixelSize: 11
        }
        SpinBox {
            id: seconds
            objectName: "timerSeconds"
            from: 0
            to: 59
            value: 0
            editable: true
            live: true
            Layout.preferredWidth: 74
            enabled: root.ready
            Accessible.name: "Custom timer seconds"
        }
        PanelButton {
            objectName: "startTimer"
            text: "Start"
            enabled: root.ready && !root.controller.busy && root.controller.unfinished < 10 && minutes.value * 60 + seconds.value > 0 && minutes.value * 60 + seconds.value <= 86400
            onClicked: root.controller.start(name.text, minutes.value * 60 + seconds.value)
        }
    }
    Text {
        visible: root.ready && root.controller.snapshot.timers.length === 0
        text: "No timers yet. Start a preset or choose a duration."
        color: Theme.muted
        font.pixelSize: 12
        Layout.fillWidth: true
        wrapMode: Text.WordWrap
    }
    ListModel {
        id: rows
    }
    function syncRows() {
        const wanted = controller.snapshot.timers || [];
        for (let i = rows.count - 1; i >= 0; i--)
            if (!wanted.some(r => r.id === rows.get(i).timerID))
                rows.remove(i);
        for (let i = 0; i < wanted.length; i++) {
            let found = -1;
            for (let j = i; j < rows.count; j++)
                if (rows.get(j).timerID === wanted[i].id) {
                    found = j;
                    break;
                }
            const t = wanted[i];
            const row = {
                timerID: t.id,
                timerName: t.name,
                timerState: t.state,
                timerRevision: t.revision,
                remaining: t.remainingMs,
                notification: t.notification || ""
            };
            if (found < 0)
                rows.insert(i, row);
            else {
                if (found !== i)
                    rows.move(found, i, 1);
                rows.set(i, row);
            }
        }
    }
    Component.onCompleted: syncRows()
    Connections {
        target: root.controller
        function onSnapshotChanged() {
            root.syncRows();
        }
    }
    Timer {
        id: revealNewTimer
        interval: 60
        onTriggered: if (root.visible && timerRows.count > 0)
            root.revealRequested(timerRows.itemAt(0))
    }
    Connections {
        target: root.controller
        function onActionSucceeded(method) {
            if (method === "timers.start" && root.visible)
                revealNewTimer.restart();
        }
    }
    Repeater {
        id: timerRows
        model: rows
        delegate: Rectangle {
            id: card
            required property string timerID
            required property string timerName
            required property string timerState
            required property double timerRevision
            required property double remaining
            required property string notification
            objectName: "timer-" + timerID
            Layout.fillWidth: true
            implicitHeight: content.implicitHeight + 24
            radius: 12
            color: Qt.alpha(Theme.surface, 0.55)
            border.color: Theme.border
            ColumnLayout {
                id: content
                x: 12
                y: 12
                width: parent.width - 24
                spacing: 8
                RowLayout {
                    Layout.fillWidth: true
                    Text {
                        text: card.timerName
                        textFormat: Text.PlainText
                        color: Theme.foreground
                        font.pixelSize: 13
                        Layout.fillWidth: true
                        wrapMode: Text.WrapAnywhere
                    }
                    Text {
                        objectName: "remainingTime"
                        text: card.timerState === "completed" ? "Done" : root.controller.formatRemaining(card.remaining)
                        color: Theme.accent
                        font.pixelSize: 18
                    }
                }
                Text {
                    text: card.timerState === "completed" ? (card.notification === "unavailable" ? "Completed · desktop alert unavailable" : card.notification === "sending" ? "Completed · requesting alert…" : "Completed") : card.timerState === "paused" ? "Paused" : card.remaining === 0 ? "Finishing…" : "Running"
                    color: Theme.muted
                    font.pixelSize: 11
                    Layout.fillWidth: true
                    wrapMode: Text.WordWrap
                }
                RowLayout {
                    PanelButton {
                        onActiveFocusChanged: if (activeFocus)
                            root.revealRequested(card)
                        objectName: "pauseTimer"
                        text: "Pause"
                        visible: card.timerState === "running"
                        enabled: root.ready && !root.controller.busy
                        onClicked: root.controller.act("pause", card.timerID, card.timerRevision)
                    }
                    PanelButton {
                        onActiveFocusChanged: if (activeFocus)
                            root.revealRequested(card)
                        objectName: "resumeTimer"
                        text: "Resume"
                        visible: card.timerState === "paused"
                        enabled: root.ready && !root.controller.busy
                        onClicked: root.controller.act("resume", card.timerID, card.timerRevision)
                    }
                    PanelButton {
                        onActiveFocusChanged: if (activeFocus)
                            root.revealRequested(card)
                        objectName: "cancelTimer"
                        text: "Cancel"
                        visible: card.timerState !== "completed"
                        enabled: root.ready && !root.controller.busy
                        onClicked: root.controller.act("cancel", card.timerID, card.timerRevision)
                    }
                    PanelButton {
                        onActiveFocusChanged: if (activeFocus)
                            root.revealRequested(card)
                        objectName: "dismissTimer"
                        text: "Dismiss"
                        visible: card.timerState === "completed"
                        enabled: root.ready && !root.controller.busy
                        onClicked: root.controller.act("dismiss", card.timerID, card.timerRevision)
                    }
                }
            }
        }
    }
    Text {
        visible: root.ready
        text: "1 second–24 hours. Up to 10 unfinished timers. The latest 20 completions are kept until dismissed."
        Layout.fillWidth: true
        wrapMode: Text.WordWrap
        font.pixelSize: 10
        color: Theme.muted
    }
}
