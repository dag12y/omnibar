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
            signal moduleEvent(string module, string event, var data)
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
        TimerController {}
    }
    TestCase {
        name: "TimerController"
        property var fake
        property var controller
        function init() {
            fake = createTemporaryObject(clientFactory, parent);
            controller = createTemporaryObject(factory, parent, {
                client: fake,
                active: true,
                pageActive: false
            });
            fake.messageReceived({
                jsonrpc: "2.0",
                id: controller.pendingId,
                result: {
                    revision: 1,
                    timers: [],
                    running: 0,
                    paused: 0,
                    completed: 0
                }
            });
            fake.messageReceived({
                jsonrpc: "2.0",
                id: controller.subscriptionID,
                result: {
                    module: "timers",
                    events: ["state"]
                }
            });
        }
        function test_visible_polling_and_hidden_events() {
            verify(controller.subscribed);
            const count = fake.socket.writes.length;
            wait(1100);
            compare(fake.socket.writes.length, count);
            controller.active = false;
            fake.moduleEvent("timers", "state", {
                revision: 2,
                timers: [],
                running: 1,
                paused: 1,
                completed: 1
            });
            compare(controller.indicator, "Timers · 2 · 1 done");
            wait(1100);
            compare(fake.socket.writes.length, count);
            controller.active = true;
            compare(fake.socket.writes[fake.socket.writes.length - 1].method, "timers.getSnapshot");
        }
        function test_start_nonce_and_exact_revision_actions() {
            controller.start(" Tea ", 300);
            const sent = fake.socket.writes[fake.socket.writes.length - 1];
            compare(sent.method, "timers.start");
            compare(sent.params.name, "Tea");
            compare(sent.params.seconds, 300);
            compare(sent.params.id.length, 32);
            fake.messageReceived({
                jsonrpc: "2.0",
                id: controller.pendingId,
                result: {
                    revision: 2,
                    timers: [],
                    running: 1,
                    paused: 0,
                    completed: 0
                }
            });
            controller.act("pause", "target", 9);
            const action = fake.socket.writes[fake.socket.writes.length - 1];
            compare(action.method, "timers.pause");
            compare(action.params.id, "target");
            compare(action.params.revision, 9);
        }
        function test_format_bounds_and_stale_events() {
            compare(controller.formatRemaining(0), "00:00");
            compare(controller.formatRemaining(1), "00:01");
            compare(controller.formatRemaining(61000), "01:01");
            compare(controller.formatRemaining(86400000), "24:00:00");
            fake.moduleEvent("timers", "state", {
                revision: 4,
                timers: [],
                running: 2,
                paused: 0,
                completed: 0
            });
            fake.moduleEvent("timers", "state", {
                revision: 3,
                timers: [],
                running: 0,
                paused: 0,
                completed: 0
            });
            compare(controller.unfinished, 2);
        }
        function test_disconnect_invalidates_subscription_and_counts() {
            fake.backendConnected = false;
            fake.status = "offline";
            verify(!controller.subscribed);
            compare(controller.indicator, "Timers");
            compare(controller.state, "offline");
            fake.backendConnected = true;
            fake.status = "connected";
            fake.handshakeSucceeded();
            compare(fake.socket.writes[fake.socket.writes.length - 1].method, "timers.getSnapshot");
        }
    }
}
