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
        id: controllerFactory
        FileShelfController {}
    }
    TestCase {
        name: "FileShelfController"
        property var fake
        property var controller
        function init() {
            fake = createTemporaryObject(clientFactory, parent);
            controller = createTemporaryObject(controllerFactory, parent, {
                client: fake
            });
        }
        function request() {
            return fake.socket.writes[fake.socket.writes.length - 1];
        }
        function reply(value, error) {
            fake.messageReceived({
                jsonrpc: "2.0",
                id: request().id,
                result: value,
                error: error
            });
        }
        function test_open_only_reads_snapshot() {
            controller.active = true;
            compare(request().method, "shelf.getSnapshot");
            reply({
                revision: 1,
                entries: [],
                can_copy: false
            });
            compare(controller.state, "ready");
            compare(fake.socket.writes.length, 1);
            controller.request("shelf.add", {
                paths: ["/tmp/fixture"]
            });
            compare(request().method, "shelf.add");
        }
        function test_hide_and_disconnect_drop_frontend_contents() {
            controller.active = true;
            reply({
                revision: 1,
                entries: [
                    {
                        id: "1",
                        path: "/tmp/fixture"
                    }
                ],
                can_copy: true
            });
            controller.active = false;
            compare(controller.snapshot.entries.length, 0);
            controller.active = true;
            compare(request().method, "shelf.getSnapshot");
            reply({
                revision: 1,
                entries: [
                    {
                        id: "1",
                        path: "/tmp/fixture"
                    }
                ],
                can_copy: true
            });
            fake.backendConnected = false;
            fake.status = "unavailable";
            compare(controller.snapshot.entries.length, 0);
            compare(controller.state, "offline");
        }
        function test_failed_open_stays_visible_across_poll() {
            controller.active = true;
            reply({
                revision: 1,
                entries: [],
                can_copy: false
            });
            controller.request("shelf.openFolder", {
                id: "1"
            });
            reply(undefined, {
                code: -32603,
                message: "Could not open folder"
            });
            controller.request("shelf.getSnapshot");
            reply({
                revision: 1,
                entries: [],
                can_copy: false
            });
            compare(controller.errorText, "Could not open folder");
        }
        function test_denied_read_and_failed_action() {
            controller.active = true;
            reply(undefined, {
                code: -32001,
                message: "Permission denied"
            });
            compare(controller.state, "denied");
            controller.request("shelf.getSnapshot");
            reply({
                revision: 1,
                entries: [],
                can_copy: false
            });
            controller.request("shelf.copyPath", {
                id: "gone"
            });
            reply(undefined, {
                code: -32001,
                message: "Write denied"
            });
            compare(controller.state, "ready");
            verify(controller.errorText.length > 0);
        }
    }
}
