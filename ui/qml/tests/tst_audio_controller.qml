import QtQuick
import QtTest
import "../components"

Item {
    Component {
        id: clientFactory
        QtObject {
            property bool handshakeComplete: true
            property bool backendConnected: true
            property string status: "connected"
            property int nextRequestId: 1
            property var modules: ["audio"]
            signal handshakeSucceeded()
            signal messageReceived(var message)
            property QtObject socket: QtObject {
                property bool connected: true
                property var writes: []
                function write(line) { writes = writes.concat([JSON.parse(line)]) }
                function flush() {}
            }
        }
    }
    Component { id: controllerFactory; AudioController {} }
    TestCase {
        name: "AudioController"
        property var fake
        property var controller
        function init() {
            fake = clientFactory.createObject(parent)
            controller = controllerFactory.createObject(parent, {client:fake})
        }
        function cleanup() { controller.destroy(); fake.destroy() }
        function lastRequest() { return fake.socket.writes[fake.socket.writes.length - 1] }
        function reply(result, error) {
            fake.messageReceived({jsonrpc:"2.0", id:lastRequest().id, result:result, error:error})
        }
        function test_demand_and_snapshot() {
            compare(fake.socket.writes.length, 0)
            controller.active = true
            compare(lastRequest().method, "module.subscribe")
            reply({module:"audio",events:[]})
            compare(lastRequest().method, "audio.getSnapshot")
            reply({revision:1, available:true, outputs:[], streams:[]})
            compare(controller.state, "ready")
            verify(!controller.busy)
            controller.active = false
            compare(lastRequest().method, "module.unsubscribe")
        }
        function test_permission_denied() {
            controller.active = true
            reply(undefined, {code:-32001,message:"Permission denied"})
            compare(controller.state, "denied")
            verify(!controller.busy)
        }
        function test_volume_requests_coalesce_without_losing_latest_value() {
            controller.active = true
            reply({module:"audio",events:[]})
            reply({revision:1,available:true,outputs:[],streams:[]})
            controller.request("audio.setVolume",{key:"one",volume:51})
            controller.request("audio.setVolume",{key:"one",volume:52})
            controller.request("audio.setVolume",{key:"one",volume:53})
            compare(lastRequest().params.volume,51)
            compare(controller.queuedRequests.length,1)
            reply({revision:2,available:true,outputs:[],streams:[]})
            verify(controller.busy)
            compare(lastRequest().params.volume,53)
            reply({revision:3,available:true,outputs:[],streams:[]})
            verify(!controller.busy)
        }
        function test_route_queue_keeps_destinations_separate_by_stream() {
            controller.active = true
            reply({module:"audio",events:[]})
            reply({revision:1,available:true,outputs:[],streams:[]})
            controller.request("audio.moveStream",{key:"one",output:"speaker"})
            controller.request("audio.moveStream",{key:"one",output:"headphones"})
            controller.request("audio.moveStream",{key:"two",output:"speaker"})
            controller.request("audio.moveStream",{key:"one",output:"hdmi"})
            compare(controller.queuedRequests.length,2)
            reply({revision:2,available:true,outputs:[],streams:[]})
            compare(lastRequest().params.key,"one")
            compare(lastRequest().params.output,"hdmi")
            reply({revision:3,available:true,outputs:[],streams:[]})
            compare(lastRequest().params.key,"two")
            compare(lastRequest().params.output,"speaker")
            reply({revision:4,available:true,outputs:[],streams:[]})
            verify(!controller.busy)
        }
        function test_old_snapshot_cannot_override_new_event() {
            controller.active = true
            controller.applySnapshot({revision:8,available:true,streams:[{volume:70}]})
            controller.applySnapshot({revision:7,available:true,streams:[{volume:20}]})
            compare(controller.snapshot.streams[0].volume, 70)
        }
        function test_disconnect_and_reconnect() {
            controller.active = true
            fake.backendConnected = false
            fake.status = "unavailable"
            compare(controller.state, "offline")
            verify(!controller.busy)
            fake.backendConnected = true
            fake.status = "connected"
            fake.handshakeSucceeded()
            compare(lastRequest().method, "module.subscribe")
            compare(controller.revision, -1)
        }
    }
}
