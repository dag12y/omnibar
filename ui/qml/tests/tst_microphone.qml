import QtQuick
import QtTest
import "../components"

Item {
    width: 400; height: 700
    property var page
    Component { id: pageFactory; MicrophonePage { width: 360; ready: true } }
    SignalSpy { id: gainSpy; target: page; signalName: "volumeRequested" }
    SignalSpy { id: muteSpy; target: page; signalName: "muteRequested" }
    SignalSpy { id: inputSpy; target: page; signalName: "inputRequested" }
    TestCase {
        name: "Microphone"
        when: windowShown
        function snapshot() {
            return {default_input:"mic_a", inputs:[
                {id:"10",name:"mic_a",description:"Desk microphone",volume:55,muted:false,monitor:false},
                {id:"11",name:"mic_b",description:"Headset microphone",volume:80,muted:true,monitor:false},
                {id:"1",name:"speakers.monitor",description:"Speakers",volume:60,muted:false,monitor:true}
            ],recordings:[{id:"20",name:"Call",input_id:"10",paused:false},{id:"21",name:"Screen sharing",input_id:"1",paused:true}]}
        }
        function init() {
            page = createTemporaryObject(pageFactory,parent,{snapshot:snapshot()})
            verify(page !== null)
            gainSpy.clear(); muteSpy.clear(); inputSpy.clear()
            waitForPolish(page); waitForRendering(page)
        }
        function test_gain_and_mute_target_default_microphone() {
            const card = findChild(page,"microphoneGain")
            const slider = findChild(card,"volume")
            slider.forceActiveFocus()
            keyClick(Qt.Key_Right)
            compare(gainSpy.count,1)
            compare(gainSpy.signalArguments[0][0],"mic_a")
            compare(gainSpy.signalArguments[0][1],56)
            const mute = findChild(card,"mute")
            mouseClick(mute,mute.width/2,mute.height/2)
            compare(muteSpy.signalArguments[0][0],"mic_a")
            compare(muteSpy.signalArguments[0][1],true)
        }
        function test_default_switch_excludes_monitors_and_preserves_levels() {
            const selector = findChild(page,"inputSelector")
            compare(selector.count,2)
            selector.forceActiveFocus(); keyClick(Qt.Key_Down)
            compare(inputSpy.signalArguments[0][0],"mic_b")
            const next = snapshot(); next.default_input = "mic_b"; page.snapshot = next
            const card = findChild(page,"microphoneGain")
            compare(card.level,80)
            verify(card.muted)
            compare(gainSpy.count,0); compare(muteSpy.count,0)
        }
        function test_recording_source_labels_and_missing_microphone() {
            compare(findChild(page,"recordingSource").text,"Desk microphone")
            const next = snapshot(); next.inputs = []; page.snapshot = next
            compare(findChild(page,"recordingSource").text,"Input unavailable")
            verify(!findChild(page,"inputSelector").enabled)
            verify(!findChild(page,"microphoneGain").visible)
            compare(gainSpy.count,0); compare(muteSpy.count,0)
        }
        function test_offline_controls_cannot_change_microphone() {
            page.ready = false
            const card = findChild(page,"microphoneGain")
            verify(!findChild(card,"volume").enabled)
            verify(!findChild(card,"mute").enabled)
            verify(!findChild(page,"inputSelector").enabled)
        }
    }
}
