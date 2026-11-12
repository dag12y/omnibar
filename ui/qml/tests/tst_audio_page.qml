import QtQuick
import QtTest
import "../components"

Item {
    width: 400; height: 600
    property var page
    Component { id: pageFactory; AudioPage { width: 360; state: "ready" } }
    SignalSpy { id: volumeSpy; target: page; signalName: "volumeRequested" }
    SignalSpy { id: routeSpy; target: page; signalName: "routeRequested" }
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
            page = createTemporaryObject(pageFactory,parent)
            verify(page !== null)
            page.state = "ready"; page.busy = false
            page.snapshot = snap([])
            waitForRendering(page)
            page.snapshot = snap([stream("one",50)])
            volumeSpy.clear(); muteSpy.clear(); routeSpy.clear()
            waitForPolish(page)
            waitForRendering(page)
        }
        function routingState(outputId) {
            const state = snap([stream("one",50)])
            state.streams[0].output_id = outputId || "1"
            state.outputs.push({id:"2",name:"headphones",description:"Headphones"})
            return state
        }
        function test_keyboard_route_and_actual_output_update() {
            page.snapshot = routingState("1")
            wait(0)
            const selector = findChild(page,"streamOutputSelector")
            compare(selector.currentIndex,0)
            selector.forceActiveFocus()
            keyClick(Qt.Key_Down)
            compare(routeSpy.count,1)
            compare(routeSpy.signalArguments[0][0],"one")
            compare(routeSpy.signalArguments[0][1],"headphones")
            page.snapshot = routingState("2")
            compare(selector.currentIndex,1)
            compare(page.snapshot.default_output,"speaker")
            compare(findChild(page,"streamOutputSelector"),selector)
        }
        function test_failed_route_restores_confirmed_output() {
            page.snapshot = routingState("1")
            wait(0)
            const selector = findChild(page,"streamOutputSelector")
            page.busy = true
            selector.forceActiveFocus()
            keyClick(Qt.Key_Down)
            compare(selector.currentIndex,1)
            page.errorText = "Output is no longer available"
            page.busy = false
            compare(selector.currentIndex,0)
            compare(routeSpy.count,1)
            page.errorText = ""
        }
        function test_refresh_keeps_popup_but_hotplug_closes_it() {
            page.snapshot = routingState("1")
            wait(0)
            const selector = findChild(page,"streamOutputSelector")
            selector.popup.open()
            tryCompare(selector.popup,"visible",true)
            page.snapshot = routingState("1")
            verify(selector.popup.visible)
            page.snapshot = snap([stream("one",50)])
            tryCompare(selector.popup,"visible",false)
            compare(selector.count,1)
            compare(routeSpy.count,0)
        }
        function test_missing_current_output_is_explicit() {
            page.snapshot = routingState("2")
            wait(0)
            const selector = findChild(page,"streamOutputSelector")
            page.snapshot = snap([Object.assign(stream("one",50),{output_id:"2"})])
            compare(selector.currentIndex,-1)
            compare(selector.displayText,"Output unavailable")
            compare(routeSpy.count,0)
        }
        function test_mute_and_keyboard_volume() {
            const mute = findChild(page,"mute")
            verify(mute !== null)
            mouseClick(mute, mute.width/2, mute.height/2)
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
