import QtQuick

SnapshotController {
    methodPrefix: "shelf"
    displayName: "File shelf"
    pollInterval: 2000
    emptySnapshot: ({
            entries: [],
            can_copy: false
        })
}
