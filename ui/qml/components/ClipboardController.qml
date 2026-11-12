import QtQuick

SnapshotController {
    methodPrefix: "clipboard"
    displayName: "Clipboard"
    emptySnapshot: ({
            entries: [],
            collecting: false,
            state: "paused",
            can_copy: false
        })
}
