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
        PreviewController {}
    }
    TestCase {
        name: "WindowPreviewController"
        property var fake
        property var controller
        function init() {
            fake = createTemporaryObject(clientFactory, parent);
            controller = createTemporaryObject(factory, parent, {
                client: fake,
                active: true,
                allowed: true,
                eligibleAddresses: ["0x1", "0x2"]
            });
        }
        function reply(id, result, error) {
            fake.messageReceived({
                jsonrpc: "2.0",
                id: id,
                result: result,
                error: error
            });
        }
        function image(address) {
            return {
                address: address,
                dataURL: "data:image/jpeg;base64,AAAA",
                width: 160,
                height: 90
            };
        }
        function test_explicit_only_and_one_inflight_capture() {
            compare(fake.socket.writes.length, 0);
            controller.request("missing");
            compare(fake.socket.writes.length, 0);
            controller.request("0x1");
            compare(fake.socket.writes[0].method, "windows.getPreview");
            compare(fake.socket.writes[0].params.address, "0x1");
            verify(controller.busy);
            controller.request("0x2");
            compare(fake.socket.writes.length, 1);
            reply(controller.pendingId, image("0x1"));
            compare(controller.imageSource, "data:image/jpeg;base64,AAAA");
            verify(!controller.busy);
            controller.request("0x2");
            compare(controller.imageSource, "");
            reply(controller.pendingId, image("0x2"));
            compare(controller.address, "0x2");
        }
        function test_hiding_drops_late_frames_and_does_not_recapture_on_show() {
            controller.request("0x1");
            const id = controller.pendingId;
            controller.active = false;
            reply(id, image("0x1"));
            compare(controller.address, "");
            compare(controller.imageSource, "");
            verify(!controller.busy);
            controller.active = true;
            compare(fake.socket.writes.length, 1);
        }
        function test_grants_scope_and_removed_targets_dispose_frames() {
            controller.allowed = false;
            controller.request("0x1");
            compare(fake.socket.writes.length, 0);
            controller.allowed = true;
            controller.request("0x1");
            reply(controller.pendingId, image("0x1"));
            controller.scope = "search";
            compare(controller.imageSource, "");
            controller.request("0x2");
            reply(controller.pendingId, image("0x2"));
            controller.eligibleAddresses = ["0x1"];
            compare(controller.imageSource, "");
            compare(controller.address, "");
            controller.request("0x1");
            reply(controller.pendingId, image("0x1"));
            controller.allowed = false;
            compare(controller.imageSource, "");
        }
        function test_failed_mismatched_and_oversized_frames_stay_empty() {
            controller.request("0x1");
            reply(controller.pendingId, undefined, {
                code: -32603,
                message: "Capture unavailable"
            });
            compare(controller.errorText, "Capture unavailable");
            compare(controller.imageSource, "");
            controller.request("0x1");
            reply(controller.pendingId, image("0x2"));
            compare(controller.imageSource, "");
            verify(controller.errorText.length > 0);
            controller.request("0x1");
            const big = image("0x1");
            big.width = 10000;
            reply(controller.pendingId, big);
            compare(controller.imageSource, "");
            controller.request("0x1");
            const huge = image("0x1");
            huge.dataURL += "A".repeat(180000);
            reply(controller.pendingId, huge);
            compare(controller.imageSource, "");
        }
        function test_reconnect_and_clear_invalidate_old_request_ids() {
            controller.request("0x1");
            const old = controller.pendingId;
            fake.backendConnected = false;
            fake.status = "unavailable";
            reply(old, image("0x1"));
            compare(controller.imageSource, "");
            fake.backendConnected = true;
            fake.status = "connected";
            controller.request("0x2");
            const next = controller.pendingId;
            reply(old, image("0x1"));
            compare(controller.pendingId, next);
            reply(next, image("0x2"));
            verify(controller.imageSource.length > 0);
            fake.handshakeSucceeded();
            compare(controller.imageSource, "");
        }
    }
}
