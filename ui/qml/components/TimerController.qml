import QtQuick
import "protocol.js" as Protocol

SnapshotController {
    id: root
    property bool pageActive: false
    property bool subscribed: false
    property int subscriptionID: -1
    readonly property int unfinished: (snapshot.running || 0) + (snapshot.paused || 0)
    readonly property string indicator: "Timers" + (unfinished ? " · " + unfinished : "") + (snapshot.completed ? " · " + snapshot.completed + " done" : "")
    methodPrefix: "timers"
    displayName: "Timers"
    pollInterval: 1000
    pollingEnabled: pageActive
    emptySnapshot: ({
            timers: [],
            running: 0,
            paused: 0,
            completed: 0,
            revision: 0
        })
    function ensureSubscription() {
        if (!client || !active || state !== "ready" || subscribed || subscriptionID >= 0 || !client.handshakeComplete || !client.socket.connected)
            return;
        subscriptionID = client.nextRequestId++;
        client.socket.write(Protocol.buildRequest(subscriptionID, "module.subscribe", {
            module: "timers",
            events: ["state"]
        }) + "\n");
        client.socket.flush();
    }
    onStateChanged: ensureSubscription()
    onSnapshotChanged: ensureSubscription()
    function start(name, seconds) {
        if (busy || state !== "ready")
            return;
        let id = "";
        for (let i = 0; i < 32; i++)
            id += Math.floor(Math.random() * 16).toString(16);
        request("timers.start", {
            id: id,
            name: name.trim() || "Timer",
            seconds: seconds
        });
    }
    function act(action, id, revision) {
        if (!busy)
            request("timers." + action, {
                id: id,
                revision: revision
            });
    }
    function formatRemaining(milliseconds) {
        const total = Math.max(0, Math.ceil(milliseconds / 1000));
        const hours = Math.floor(total / 3600);
        const minutes = Math.floor(total / 60) % 60;
        const seconds = total % 60;
        const pad = value => String(value).padStart(2, "0");
        return hours ? hours + ":" + pad(minutes) + ":" + pad(seconds) : pad(minutes) + ":" + pad(seconds);
    }
    Connections {
        target: root.client
        function onHandshakeSucceeded() {
            root.subscribed = false;
            root.subscriptionID = -1;
        }
        function onStatusChanged() {
            if (!root.client.backendConnected) {
                root.subscribed = false;
                root.subscriptionID = -1;
            }
        }
        function onMessageReceived(message) {
            const parsed = Protocol.classify(message);
            if (root.subscriptionID < 0 || parsed.kind !== "response" || parsed.id !== root.subscriptionID)
                return;
            root.subscriptionID = -1;
            root.subscribed = !parsed.error;
            if (parsed.error)
                root.errorText = "Timer live updates unavailable. Reopen to refresh.";
        }
        function onModuleEvent(module, event, data) {
            if (module !== "timers" || event !== "state" || !data || data.revision < root.revision)
                return;
            root.revision = data.revision;
            root.snapshot = data;
            root.state = "ready";
        }
    }
}
