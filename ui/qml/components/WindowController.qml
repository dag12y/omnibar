import QtQuick

SnapshotController {
    signal windowFocused
    onActionSucceeded: method => {
        if (method === "windows.focus")
            windowFocused();
    }
    methodPrefix: "windows"
    displayName: "Window switcher"
    pollInterval: 1500
    requestTimeout: pendingMethod === "windows.restoreLayout" ? 15000 : 8000
    emptySnapshot: ({
            windows: [],
            truncated: false
        })
}
