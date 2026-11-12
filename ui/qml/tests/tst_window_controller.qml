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
            signal handshakeSucceeded
            signal messageReceived(var message)
            property QtObject socket: QtObject {
                property bool connected: true
                property var writes: []
                function write(line) {
                    writes = writes.concat([JSON.parse(line)]);
                }
                function flush() {
                }
            }
        }
    }
    Component {
        id: factory
        WindowController {}
    }
    TestCase {
        name: "WindowController"
        property var fake
        property var controller
        SignalSpy {
            id: focused
            target: parent.controller
            signalName: "windowFocused"
        }
        function init() {
            fake = createTemporaryObject(clientFactory, parent);
            controller = createTemporaryObject(factory, parent, {
                client: fake
            });
            focused.clear();
        }
        function reply(result, error) {
            const requests = fake.socket.writes;
            fake.messageReceived({
                jsonrpc: "2.0",
                id: requests[requests.length - 1].id,
                result: result,
                error: error
            });
        }
        function test_focus_success_signal_only_after_confirmed_reply() {
            controller.active = true;
            compare(fake.socket.writes[0].method, "windows.getSnapshot");
            reply({
                revision: 1,
                windows: []
            });
            compare(focused.count, 0);
            controller.request("windows.focus", {
                address: "0x1"
            });
            compare(focused.count, 0);
            reply(undefined, {
                code: -32603,
                message: "Window no longer available"
            });
            compare(focused.count, 0);
            compare(controller.state, "ready");
            controller.request("windows.focus", {
                address: "0x2"
            });
            reply({
                revision: 2,
                windows: [
                    {
                        address: "0x2",
                        active: true
                    }
                ]
            });
            compare(focused.count, 1);
        }
        function test_hide_clears_window_titles() {
            controller.active = true;
            reply({
                revision: 1,
                windows: [
                    {
                        address: "0x1",
                        title: "Synthetic title"
                    }
                ]
            });
            controller.active = false;
            compare(controller.snapshot.windows.length, 0);
        }
        function test_permission_error_and_disconnect() {
            controller.active = true;
            reply(undefined, {
                code: -32001,
                message: "Grant required"
            });
            compare(controller.state, "denied");
            fake.backendConnected = false;
            fake.status = "unavailable";
            compare(controller.state, "offline");
            compare(focused.count, 0);
        }
    }
}
