import QtQuick
import QtTest
import "../components"

Item {
    width: 400; height: 200
    KeepAwakeCard { id: card; width: 360; connected: true; available: true }
    SignalSpy { id: startSpy; target: card; signalName: "startRequested" }
    SignalSpy { id: stopSpy; target: card; signalName: "stopRequested" }
    TestCase {
        name: "KeepAwakeControls"
        when: windowShown
        function init() {
            card.active = false; card.available = true; card.busy = false
            card.connected = true
            startSpy.clear(); stopSpy.clear()
        }
        function test_start_preset() {
            const start = findChild(card, "start15")
            verify(start !== null)
            mouseClick(start)
            compare(startSpy.count, 1)
            compare(startSpy.signalArguments[0][0], 900)
        }
        function test_stop_session() {
            card.active = true
            mouseClick(findChild(card, "stopSession"))
            compare(stopSpy.count, 1)
        }
        function test_offline_and_denied_cannot_start() {
            const start = findChild(card, "start15")
            card.connected = false
            verify(!start.enabled)
            card.connected = true; card.available = false
            verify(!start.enabled)
            mouseClick(start)
            compare(startSpy.count, 0)
        }
    }
}
