import QtQuick

SnapshotController {
    signal appLaunched
    onActionSucceeded: method => {
        if (method === "apps.launch")
            appLaunched();
    }
    methodPrefix: "apps"
    displayName: "Application launcher"
    pollInterval: 10000
    emptySnapshot: ({
            apps: [],
            truncated: false
        })
}
