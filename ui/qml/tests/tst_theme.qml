import QtQuick
import QtTest
import "../components"

TestCase {
    name: "Theme"
    property var savedColors
    function init() { savedColors = Theme.colors }
    function cleanup() { Theme.colors = savedColors }
    function test_light_and_dark_theme_switch() {
        Theme.load('background = "#fafafa"\nforeground = "#151515"\naccent = "#2255aa"')
        compare(Theme.background.toString(), "#fafafa")
        compare(Theme.foreground.toString(), "#151515")
        Theme.load('background = "#151515"\nforeground = "#fafafa"\naccent = "#aabbff"')
        compare(Theme.background.toString(), "#151515")
        compare(Theme.foreground.toString(), "#fafafa")
        compare(Theme.accent.toString(), "#aabbff")
    }
    function test_missing_or_invalid_palette_uses_fallback() {
        Theme.load('background = "not a color"\n# accent = "#ffffff"')
        compare(Theme.background.toString(), "#181c25")
        compare(Theme.accent.toString(), "#89b4fa")
    }
}
