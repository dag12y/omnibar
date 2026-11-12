import QtQuick
import QtTest
import "../components"

Item {
    width: 400
    height: 200
    SummaryCard {
        id: card
        width: 180; height: 150
        label: "Processor"; value: "12%"
    }
    PanelButton {
        id: button
        x: 200
        text: "Refresh"
    }
    SignalSpy { id: cardSpy; target: card; signalName: "activated" }
    SignalSpy { id: buttonSpy; target: button; signalName: "clicked" }
    TestCase {
        name: "PanelControls"
        when: windowShown
        function init() { cardSpy.clear(); buttonSpy.clear() }
        function test_card_keyboard_activation() {
            card.forceActiveFocus()
            keyClick(Qt.Key_Space)
            compare(cardSpy.count, 1)
            keyClick(Qt.Key_Return)
            compare(cardSpy.count, 2)
        }
        function test_tab_navigation() {
            card.forceActiveFocus()
            keyClick(Qt.Key_Tab)
            verify(button.activeFocus)
            keyClick(Qt.Key_Space)
            compare(buttonSpy.count, 1)
        }
        function test_pointer_activation() {
            mouseClick(card, 50, 50)
            compare(cardSpy.count, 1)
        }
    }
}
