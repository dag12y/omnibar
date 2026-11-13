import QtQuick
import "protocol.js" as Protocol

Item {
    id: root
    required property var client
    property bool active: false
    property bool allowed: false
    property string scope: ""
    property var eligibleAddresses: []
    property string address: ""
    property string imageSource: ""
    property string errorText: ""
    property bool busy: false
    property int pendingId: -1

    function clear() {
        pendingId = -1;
        address = "";
        imageSource = "";
        errorText = "";
        busy = false;
        timeout.stop();
    }
    function request(target) {
        if (!active || !allowed || busy || !client.handshakeComplete || !client.socket.connected || !eligibleAddresses.includes(target))
            return;
        clear();
        address = target;
        pendingId = client.nextRequestId++;
        busy = true;
        client.socket.write(Protocol.buildRequest(pendingId, "windows.getPreview", {
            address: target
        }) + "\n");
        client.socket.flush();
        timeout.restart();
    }
    onActiveChanged: if (!active)
        clear()
    onAllowedChanged: if (!allowed)
        clear()
    onScopeChanged: clear()
    onEligibleAddressesChanged: if (!eligibleAddresses.includes(address))
        clear()
    Timer {
        id: timeout
        interval: 5000
        onTriggered: {
            root.pendingId = -1;
            root.busy = false;
            root.errorText = "Preview timed out. Try Preview again.";
        }
    }
    Connections {
        target: root.client
        function onHandshakeSucceeded() {
            root.clear();
        }
        function onStatusChanged() {
            if (!root.client.backendConnected)
                root.clear();
        }
        function onMessageReceived(message) {
            const parsed = Protocol.classify(message);
            if (parsed.kind !== "response" || root.pendingId < 0 || parsed.id !== root.pendingId)
                return;
            root.pendingId = -1;
            root.busy = false;
            timeout.stop();
            if (!root.active || !root.allowed) {
                root.clear();
                return;
            }
            if (parsed.error) {
                root.errorText = parsed.error.message || "Preview unavailable.";
                return;
            }
            const result = parsed.result;
            if (!result || result.address !== root.address || typeof result.dataURL !== "string" || !result.dataURL.startsWith("data:image/jpeg;base64,") || result.dataURL.length > 180000 || !Number.isInteger(result.width) || !Number.isInteger(result.height) || result.width < 1 || result.width > 320 || result.height < 1 || result.height > 200) {
                root.errorText = "Preview unavailable.";
                return;
            }
            root.imageSource = result.dataURL;
        }
    }
}
