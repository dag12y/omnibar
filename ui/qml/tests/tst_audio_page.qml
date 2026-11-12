import QtQuick
import QtTest
import "../components"

Item {
    width: 400; height: 600
    AudioPage { id: page; width: 360; state: "ready" }
    SignalSpy { id: volumeSpy; target: page; signalName: "volumeRequested" }
    SignalSpy { id: muteSpy; target: page; signalName: "muteRequested" }
    TestCase {
        name: "AudioPage"
        when: windowShown
        function snap(streams) {
            return {available:true,default_output:"speaker", outputs:[{id:"1",name:"speaker",description:"Speakers"}], streams:streams}
        }
        function stream(key, level) {
            return {key:key, name:"Music", media:"Song", icon:"", volume:level, muted:false, paused:false, output_id:"1"}
        }
        function init() {
            page.state = "ready"; page.busy = false
            page.snapshot = snap([])
            wait(0)
            page.snapshot = snap([stream("one",50)])
            volumeSpy.clear(); muteSpy.clear()
            wait(0)
        }
        function test_mute_and_keyboard_volume() {
            const mute = findChild(page,"mute")
            verify(mute !== null)
            mouseClick(mute)
            compare(muteSpy.signalArguments[0][0], "one")
            compare(muteSpy.signalArguments[0][1], true)
            const slider = findChild(page,"volume")
            slider.forceActiveFocus()
            keyClick(Qt.Key_Right)
            compare(volumeSpy.signalArguments[0][0], "one")
            compare(volumeSpy.signalArguments[0][1], 51)
        }
        function test_pending_action_keeps_keyboard_focus_and_local_value() {
            const slider = findChild(page,"volume")
            slider.forceActiveFocus()
            keyClick(Qt.Key_Right)
            page.busy = true
            page.snapshot = snap([stream("one",20)])
            verify(slider.activeFocus)
            verify(slider.enabled)
            compare(slider.value,51)
            page.busy = false
            compare(slider.value,20)
        }
        function test_drag_commits_once_and_survives_refresh() {
            const slider = findChild(page,"volume")
            mousePress(slider, slider.width / 2, slider.height / 2)
            mouseMove(slider, slider.width * 0.7, slider.height / 2)
            verify(slider.pressed)
            const desired = slider.value
            page.snapshot = snap([stream("one",10)])
            compare(findChild(page,"volume"),slider)
            compare(slider.value,desired)
            compare(volumeSpy.count,0)
            mouseRelease(slider, slider.width * 0.7, slider.height / 2)
            compare(volumeSpy.count,1)
            compare(volumeSpy.signalArguments[0][1],Math.round(desired))
        }
        function test_refresh_preserves_delegate_while_stream_exists() {
            const slider = findChild(page,"volume")
            slider.forceActiveFocus()
            page.snapshot = snap([stream("one",60)])
            compare(findChild(page,"volume"),slider)
            verify(slider.activeFocus)
            compare(slider.value,60)
        }
        function test_disappearing_stream_and_unavailable_service() {
            page.snapshot = snap([])
            compare(findChild(page,"volume"),null)
            page.snapshot = snap([stream("new",30)])
            page.state = "unavailable"
            verify(!findChild(page,"volume").enabled)
            verify(!findChild(page,"mute").enabled)
        }
    }
}
