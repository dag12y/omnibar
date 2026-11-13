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
        AppController {}
    }
    TestCase {
        name: "AppController"
        property var fake
        property var controller
        SignalSpy {
            id: launched
            target: parent.controller
            signalName: "appLaunched"
        }
        function init() {
            fake = createTemporaryObject(clientFactory, parent);
            controller = createTemporaryObject(factory, parent, {
                client: fake
            });
            launched.clear();
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
        function test_launch_signal_only_after_accepted_reply() {
            controller.active = true;
            compare(fake.socket.writes[0].method, "apps.getSnapshot");
            reply({
                revision: 1,
                apps: []
            });
            compare(launched.count, 0);
            controller.request("apps.launch", {
                id: "alpha.desktop"
            });
            compare(launched.count, 0);
            reply(undefined, {
                code: -32603,
                message: "App no longer available"
            });
            compare(launched.count, 0);
            compare(controller.state, "ready");
            controller.request("apps.launch", {
                id: "beta.desktop"
            });
            reply({
                revision: 2,
                apps: [
                    {
                        id: "beta.desktop",
                        active: true
                    }
                ]
            });
            compare(launched.count, 1);
        }
        function test_hide_clears_app_catalog() {
            controller.active = true;
            reply({
                revision: 1,
                apps: [
                    {
                        id: "alpha.desktop",
                        name: "Synthetic app"
                    }
                ]
            });
            controller.active = false;
            compare(controller.snapshot.apps.length, 0);
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
            compare(launched.count, 0);
        }
    }
}
