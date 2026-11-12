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
    emptySnapshot: ({
            windows: [],
            truncated: false
        })
}
