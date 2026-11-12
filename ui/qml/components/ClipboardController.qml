import QtQuick
import "protocol.js" as Protocol

Item {
    id: root
    required property var client
    property bool active: false
    property var snapshot: ({
            entries: [],
            collecting: false,
            state: "paused",
            can_copy: false
        })
    property string state: "pending"
    property string errorText: ""
    property bool busy: false
    property int pendingId: -1
    property string pendingMethod: ""
    property var queue: []
    property double revision: -1
    function clearCache() {
        snapshot = {
            entries: [],
            collecting: false,
            state: "paused",
            can_copy: false
        };
        revision = -1;
    }
    function request(method, params) {
        if (!active || !client.handshakeComplete || !client.socket.connected)
            return;
        if (busy) {
            if (method !== "clipboard.getSnapshot")
                queue = queue.concat([
                    {
                        method: method,
                        params: params || {}
                    }
                ]);
            return;
        }
        send(method, params);
    }
    function send(method, params) {
        pendingId = client.nextRequestId++;
        pendingMethod = method;
        busy = true;
        if (method !== "clipboard.getSnapshot")
            errorText = "";
        client.socket.write(Protocol.buildRequest(pendingId, method, params || {}) + "\n");
        client.socket.flush();
        timeout.restart();
    }
    onActiveChanged: {
        if (active) {
            state = "pending";
            request("clipboard.getSnapshot");
        } else {
            pendingId = -1;
            queue = [];
            busy = false;
            timeout.stop();
            clearCache();
        }
    }
    Timer {
        interval: 1000
        repeat: true
        running: root.active && root.client.backendConnected && root.state !== "denied" && root.state !== "disabled"
        onTriggered: root.request("clipboard.getSnapshot")
    }
    Timer {
        id: timeout
        interval: 8000
        onTriggered: {
            root.busy = false;
            root.pendingId = -1;
            root.queue = [];
            root.errorText = "Clipboard request timed out. Try Refresh.";
        }
    }
    Connections {
        target: root.client
        function onHandshakeSucceeded() {
            root.clearCache();
            if (root.active) {
                root.state = "pending";
                root.request("clipboard.getSnapshot");
            }
        }
        function onStatusChanged() {
            if (!root.client.backendConnected) {
                root.pendingId = -1;
                root.busy = false;
                root.queue = [];
                timeout.stop();
                root.clearCache();
                root.state = "offline";
            }
        }
        function onMessageReceived(message) {
            const parsed = Protocol.classify(message);
            if (parsed.kind !== "response" || parsed.id !== root.pendingId)
                return;
            const method = root.pendingMethod;
            root.pendingId = -1;
            timeout.stop();
            if (parsed.error) {
                root.errorText = parsed.error.message;
                if (method === "clipboard.getSnapshot")
                    root.state = parsed.error.code === -32001 ? "denied" : parsed.error.code === -32002 ? "disabled" : "unavailable";
                root.queue = [];
                root.busy = false;
            } else {
                if (parsed.result && parsed.result.revision >= root.revision) {
                    root.revision = parsed.result.revision;
                    root.snapshot = parsed.result;
                    if (root.state !== "ready")
                        root.errorText = "";
                    root.state = "ready";
                }
                if (root.queue.length > 0) {
                    const next = root.queue[0];
                    root.queue = root.queue.slice(1);
                    root.send(next.method, next.params);
                } else
                    root.busy = false;
            }
        }
    }
}
