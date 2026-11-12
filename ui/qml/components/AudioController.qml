import QtQuick
import "protocol.js" as Protocol

// Own audio demand and requests on the shared backend connection. Keep Awake
// subscriptions remain independent of the selected page.
Item {
    id: root
    required property var client
    property bool active: false
    property var snapshot: ({
            available: false,
            outputs: [],
            streams: []
        })
    property string state: "pending"
    onStateChanged: console.info("[omnibar-ui] audio state:", state)
    property string errorText: ""
    property bool busy: false
    property var queuedRequests: []
    property int pendingId: -1
    property int subscriptionId: -1
    property double revision: -1

    function applySnapshot(value) {
        if (!value || typeof value !== "object" || !Number.isFinite(Number(value.revision)) || Number(value.revision) < revision)
            return;
        revision = Number(value.revision);
        snapshot = value;
        state = value.available ? "ready" : "unavailable";
    }
    function request(method, params) {
        if (!active || !client.handshakeComplete || !client.socket.connected)
            return;
        if (busy) {
            const queue = queuedRequests.slice();
            const key = (params || {}).key || "";
            const index = queue.findIndex(item => item.method === method && ((item.params || {}).key || "") === key);
            const item = {
                method: method,
                params: params || {}
            };
            if (index < 0)
                queue.push(item);
            else
                queue[index] = item;
            queuedRequests = queue;
            return;
        }
        dispatchRequest(method, params);
    }
    function dispatchRequest(method, params) {
        pendingId = client.nextRequestId++;
        busy = true;
        errorText = "";
        client.socket.write(Protocol.buildRequest(pendingId, method, params || {}) + "\n");
        client.socket.flush();
        timeout.restart();
    }
    function completeRequest() {
        if (active && state === "ready" && queuedRequests.length > 0) {
            const next = queuedRequests[0];
            queuedRequests = queuedRequests.slice(1);
            dispatchRequest(next.method, next.params);
        } else {
            queuedRequests = [];
            busy = false;
        }
    }
    function subscribe() {
        if (!active || !client.handshakeComplete || !client.socket.connected)
            return;
        if (client.modules.indexOf("audio") < 0) {
            state = "unavailable";
            return;
        }
        errorText = "";
        subscriptionId = client.nextRequestId++;
        state = "pending";
        client.socket.write(Protocol.buildRequest(subscriptionId, "module.subscribe", {
            module: "audio"
        }) + "\n");
        client.socket.flush();
    }
    function syncDemand() {
        if (!client.handshakeComplete || !client.socket.connected)
            return;
        if (active)
            subscribe();
        else {
            subscriptionId = -1;
            queuedRequests = [];
            pendingId = -1;
            busy = false;
            timeout.stop();
            client.socket.write(Protocol.buildRequest(client.nextRequestId++, "module.unsubscribe", {
                module: "audio"
            }) + "\n");
            client.socket.flush();
            console.info("[omnibar-ui] audio paused");
        }
    }
    onActiveChanged: syncDemand()
    Timer {
        id: timeout
        interval: 8000
        onTriggered: {
            root.busy = false;
            root.queuedRequests = [];
            root.pendingId = -1;
            root.errorText = "Audio request timed out. Refresh to check the current state.";
        }
    }
    Connections {
        target: root.client
        function onHandshakeSucceeded() {
            root.revision = -1;
            root.syncDemand();
        }
        function onStatusChanged() {
            if (!root.client.backendConnected) {
                root.state = "offline";
                root.subscriptionId = -1;
                root.pendingId = -1;
                root.queuedRequests = [];
                root.busy = false;
                timeout.stop();
            }
        }
        function onMessageReceived(message) {
            const parsed = Protocol.classify(message);
            if (parsed.kind === "event" && parsed.module === "audio") {
                if (root.active && parsed.event === "snapshot")
                    root.applySnapshot(parsed.data);
                return;
            }
            if (parsed.kind !== "response")
                return;
            if (parsed.id === root.subscriptionId) {
                root.subscriptionId = -1;
                if (!root.active)
                    return;
                if (parsed.error) {
                    root.state = parsed.error.code === -32001 ? "denied" : "unavailable";
                    root.errorText = parsed.error.message;
                } else {
                    console.info("[omnibar-ui] subscribed to audio");
                    root.request("audio.getSnapshot");
                }
            } else if (parsed.id === root.pendingId) {
                root.pendingId = -1;
                timeout.stop();
                if (parsed.error) {
                    root.errorText = parsed.error.message;
                    root.queuedRequests = [];
                    root.busy = false;
                } else {
                    root.applySnapshot(parsed.result);
                    console.info("[omnibar-ui] audio snapshot received");
                    root.completeRequest();
                }
            }
        }
    }
}
