import QtQuick
import Quickshell
import Quickshell.Io

Item {
    FileView {
        path: (Quickshell.env("XDG_STATE_HOME") || Quickshell.env("HOME") + "/.local/state")
            + "/omarchy/current/theme/colors.toml"
        watchChanges: true
        printErrors: false
        onFileChanged: reload()
        onLoaded: Theme.load(text())
    }
}
