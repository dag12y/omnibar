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
        ClipboardController {}
    }
    TestCase {
        name: "ClipboardController"
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
        function test_open_does_not_start_collection() {
            controller.active = true;
            compare(request().method, "clipboard.getSnapshot");
            reply({
                revision: 1,
                entries: [],
                collecting: false,
                state: "paused",
                can_copy: false
            });
            compare(controller.state, "ready");
            compare(fake.socket.writes.length, 1);
            controller.request("clipboard.setCollecting", {
                collecting: true
            });
            compare(request().method, "clipboard.setCollecting");
        }
        function test_hide_and_disconnect_drop_frontend_contents() {
            controller.active = true;
            reply({
                revision: 1,
                entries: [
                    {
                        id: "1",
                        text: "fixture"
                    }
                ],
                collecting: true,
                state: "watching",
                can_copy: true
            });
            controller.active = false;
            compare(controller.snapshot.entries.length, 0);
            controller.active = true;
            compare(request().method, "clipboard.getSnapshot");
            reply({
                revision: 1,
                entries: [
                    {
                        id: "1",
                        text: "fixture"
                    }
                ],
                collecting: true,
                state: "watching",
                can_copy: true
            });
            fake.backendConnected = false;
            fake.status = "unavailable";
            compare(controller.snapshot.entries.length, 0);
            compare(controller.state, "offline");
        }
        function test_failed_save_stays_visible_across_poll() {
            controller.active = true;
            reply({
                revision: 1,
                entries: [],
                collecting: false,
                state: "paused",
                can_copy: false
            });
            controller.request("clipboard.setFavorite", {
                id: "1",
                favorite: true
            });
            reply(undefined, {
                code: -32603,
                message: "Could not write favorites"
            });
            controller.request("clipboard.getSnapshot");
            reply({
                revision: 1,
                entries: [],
                collecting: false,
                state: "paused",
                can_copy: false
            });
            compare(controller.errorText, "Could not write favorites");
        }
        function test_denied_read_and_failed_action() {
            controller.active = true;
            reply(undefined, {
                code: -32001,
                message: "Permission denied"
            });
            compare(controller.state, "denied");
            controller.request("clipboard.getSnapshot");
            reply({
                revision: 1,
                entries: [],
                collecting: false,
                state: "paused",
                can_copy: false
            });
            controller.request("clipboard.copy", {
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
