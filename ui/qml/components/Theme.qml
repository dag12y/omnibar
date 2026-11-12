pragma Singleton
import QtQuick

QtObject {
    id: root
    property var colors: ({})
    readonly property color background: colors.background || "#181c25"
    readonly property color foreground: colors.foreground || "#e4e9f4"
    readonly property color accent: colors.accent || "#89b4fa"
    readonly property color muted: colors.dark_foreground || "#929db2"
    readonly property color surface: colors.lighter_background || "#242b38"
    readonly property color urgent: colors.red || "#f38ba8"
    readonly property color border: Qt.alpha(foreground, 0.10)

    function load(raw) {
        const result = {}
        for (const line of raw.split("\n")) {
            const match = line.match(/^\s*([a-z_]+)\s*=\s*["'](#[a-fA-F0-9]{6})["']/)
            if (match) result[match[1]] = match[2]
        }
        colors = result
    }
}
