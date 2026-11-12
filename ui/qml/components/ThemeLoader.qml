import QtQuick
import Quickshell
import Quickshell.Io

Item {
    id: root
    property bool active: true
    property string lastRaw: ""
    onActiveChanged: {
        if (active)
            themeFile.reload();
    }
    // A theme switch can replace the parent symlink rather than the watched
    // file. Recheck while visible as well as responding to file notifications.
    Timer {
        interval: 3000
        repeat: true
        running: root.active
        onTriggered: themeFile.reload()
    }
    FileView {
        id: themeFile
        path: (Quickshell.env("XDG_STATE_HOME") || Quickshell.env("HOME") + "/.local/state") + "/omarchy/current/theme/colors.toml"
        watchChanges: true
        printErrors: false
        onFileChanged: reload()
        onLoaded: {
            const raw = text();
            if (raw !== root.lastRaw) {
                root.lastRaw = raw;
                Theme.load(raw);
            }
        }
    }
}
