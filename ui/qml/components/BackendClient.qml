import QtQuick
import Quickshell
import Quickshell.Io
import "backoff.js" as Backoff
import "protocol.js" as Protocol

// BackendClient owns the Unix socket connection to the Omnibar backend.
//
// It connects to $XDG_RUNTIME_DIR/omnibar/omnibar.sock, performs the
// client.handshake exchange, subscribes to every module's events, and
// reconnects with exponential backoff when the backend is unavailable. It
// never terminates the application when the backend is missing.
//
// Subscriptions live in the backend's memory, so they are made again after
// every successful handshake (ADR-0009). After a module is subscribed, the
// UI fetches its current snapshot with system.getSnapshot and then applies
// live cpu/memory events as they arrive (M11).
Item {
    id: root

    readonly property string protocolVersion: "omnibar.ipc/v1"
    readonly property string clientId: "omnibar-ui"

    readonly property string socketPath: {
        const runtimeDir = Quickshell.env("XDG_RUNTIME_DIR")
        if (!runtimeDir || String(runtimeDir).length === 0)
            return ""
        return String(runtimeDir) + "/omnibar/omnibar.sock"
    }

    // "connected", "connecting", or "unavailable".
    property string status: "connecting"

    property string lastError: ""
    property bool handshakeComplete: false
    property string backendVersion: ""
    property var modules: []

    property int retryAttempt: 0
    property int retryDelayMs: Backoff.delayForAttempt(0)
    property int pendingHandshakeId: -1
    property int nextRequestId: 1

    // Per-module subscription state: "pending", "subscribed", "denied",
    // "unavailable", or "failed". Cleared whenever the connection is lost.
    property var subscriptions: ({})

    // Request id -> module name for subscribe requests awaiting a response.
    property var pendingSubscriptions: ({})

    // Per-module snapshot data from system.getSnapshot and live events.
    property var snapshots: ({})

    // Per-module snapshot state: "pending", "fresh", "stale", "denied",
    // "unavailable", or "failed".
    property var snapshotStates: ({})

    // Request id -> module name for snapshot requests awaiting a response.
    property var pendingSnapshots: ({})

    readonly property bool backendConnected: status === "connected"
    readonly property bool connecting: status === "connecting"
    readonly property bool backendUnavailable: status === "unavailable"
    readonly property bool hasCachedData: backendVersion.length > 0 || modules.length > 0
    readonly property int retryDelaySeconds: Math.round(retryDelayMs / 1000)

    signal handshakeSucceeded()
    signal messageReceived(var message)

    // A module.event notification from the backend.
    signal moduleEvent(string module, string event, var data)

    // A module snapshot, from system.getSnapshot or a live event.
    signal snapshotReceived(string module, var data)

    property Socket socket: Socket {
        id: socket
        path: root.socketPath

        parser: SplitParser {
            splitMarker: "\n"
            onRead: function (line) {
                root.handleLine(line)
            }
        }

        onConnectionStateChanged: root.handleConnectionStateChanged()
        onError: function (error) {
            root.handleSocketError(error)
        }
    }

    property Timer retryTimer: Timer {
        interval: root.retryDelayMs
        repeat: false
        onTriggered: root.connectToBackend()
    }

    property Timer handshakeTimer: Timer {
        interval: 5000
        repeat: false
        onTriggered: root.failConnection("Handshake timed out")
    }

    Component.onCompleted: root.connectToBackend()

    onStatusChanged: {
        console.info("[omnibar-ui] status:", status)
    }

    function connectToBackend() {
        retryTimer.stop()

        if (socketPath.length === 0) {
            lastError = "XDG_RUNTIME_DIR is not set"
            status = "unavailable"
            scheduleRetry()
            return
        }

        if (socket.connected)
            return

        status = "connecting"
        handshakeComplete = false
        lastError = ""
        socket.connected = true
    }

    // User-initiated retry. Resets the backoff so the next automatic
    // retry after a new failure starts at one second again.
    function retryNow() {
        retryTimer.stop()
        retryAttempt = 0
        retryDelayMs = Backoff.delayForAttempt(0)
        connectToBackend()
    }

    // Re-run the handshake on the current connection. This is the
    // backend-dependent action exposed by the UI; it does nothing while
    // the backend is unavailable.
    function refreshHandshake() {
        if (!socket.connected) {
            lastError = "Backend is not connected"
            return
        }
        sendHandshake()
    }

    function sendHandshake() {
        if (!socket.connected)
            return

        const requestId = nextRequestId++
        pendingHandshakeId = requestId
        handshakeComplete = false

        if (status !== "connected")
            status = "connecting"

        const request = JSON.stringify({
            jsonrpc: "2.0",
            id: requestId,
            method: "client.handshake",
            params: {
                protocol: protocolVersion,
                client_id: clientId
            }
        })

        socket.write(request + "\n")
        socket.flush()
        handshakeTimer.restart()
    }

    function handleConnectionStateChanged() {
        if (socket.connected)
            handleConnected()
        else
            handleDisconnected()
    }

    function resetSubscriptions() {
        subscriptions = ({})
        pendingSubscriptions = ({})
        snapshotStates = ({})
        pendingSnapshots = ({})
    }

    function handleConnected() {
        retryTimer.stop()
        resetSubscriptions()
        handshakeComplete = false
        lastError = ""
        status = "connecting"
        sendHandshake()
    }

    function handleDisconnected() {
        handshakeTimer.stop()
        resetSubscriptions()
        pendingHandshakeId = -1
        handshakeComplete = false

        // Mark every snapshot stale: keep the last values but show them
        // as stale until the backend returns and re-subscribes.
        var staleStates = {}
        for (var module in snapshots) {
            staleStates[module] = "stale"
        }
        snapshotStates = staleStates
        if (Object.keys(snapshots).length > 0)
            console.info("[omnibar-ui] snapshots marked stale")

        if (status !== "unavailable") {
            lastError = "Connection to backend lost"
            status = "unavailable"
        }

        scheduleRetry()
    }

    function handleSocketError(error) {
        handshakeTimer.stop()
        resetSubscriptions()
        pendingHandshakeId = -1
        handshakeComplete = false
        lastError = describeSocketError(error)
        status = "unavailable"
        console.warn("[omnibar-ui] socket error:", error, lastError)
        scheduleRetry()
    }

    function handleLine(line) {
        if (!line || line.trim().length === 0)
            return

        let message
        try {
            message = JSON.parse(line)
        } catch (error) {
            console.warn("[omnibar-ui] ignoring malformed message:", error)
            return
        }

        if (!message || typeof message !== "object")
            return

        messageReceived(message)

        const parsed = Protocol.classify(message)
        if (parsed.kind === "event") {
            moduleEvent(parsed.module, parsed.event, parsed.data)
            root.applyModuleEvent(parsed.module, parsed.event, parsed.data)
            return
        }
        if (parsed.kind === "response" && pendingSubscriptions[parsed.id] !== undefined) {
            handleSubscribeResponse(parsed)
            return
        }
        if (parsed.kind === "response" && pendingSnapshots[parsed.id] !== undefined) {
            handleSnapshotResponse(parsed)
            return
        }

        if (message.id === undefined || message.id !== pendingHandshakeId)
            return

        handshakeTimer.stop()
        pendingHandshakeId = -1

        if (message.error) {
            failConnection("Handshake failed: " + (message.error.message || "unknown error"))
            return
        }

        const result = message.result
        if (!result || result.protocol !== protocolVersion) {
            failConnection("Unexpected handshake response")
            return
        }

        backendVersion = result.version ? String(result.version) : ""
        modules = Array.isArray(result.modules) ? result.modules.map(String) : []
        handshakeComplete = true
        retryAttempt = 0
        retryDelayMs = Backoff.delayForAttempt(0)
        lastError = ""
        status = "connected"

        console.info("[omnibar-ui] handshake complete; backend", backendVersion,
                     "modules:", modules.join(", "))
        subscribeToModules()
        handshakeSucceeded()
    }

    // Merge a live module.event into the stored snapshot so the UI can
    // keep showing current values between snapshots.
    function applyModuleEvent(module, event, data) {
        var snap = snapshots[module]
        if (!snap || typeof snap !== "object") {
            snap = {}
        }
        if (event === "cpu") {
            snap.cpu = data
        } else if (event === "memory") {
            snap.memory = data
            if (data.swap_total !== undefined) {
                snap.swap = { total: data.swap_total, used: data.swap_used }
            }
        } else if (event === "disk") {
            snap.disk = data
        }
        snap.sampled_at = new Date().toISOString()
        snapshots = Protocol.withState(snapshots, module, snap)
        snapshotStates = Protocol.withState(snapshotStates, module, "fresh")
        snapshotReceived(module, snap)
    }

    // Subscribe to every event of every module the backend reported. Safe
    // to repeat: subscribing is idempotent on the backend.
    function subscribeToModules() {
        if (!socket.connected)
            return

        const plan = Protocol.subscriptionPlan(modules)
        for (let i = 0; i < plan.length; i++) {
            const module = plan[i]
            const requestId = nextRequestId++

            pendingSubscriptions = Protocol.withState(pendingSubscriptions, requestId, module)
            if (subscriptions[module] === undefined)
                subscriptions = Protocol.withState(subscriptions, module, Protocol.Pending)

            socket.write(Protocol.buildRequest(requestId, "module.subscribe", { module: module }) + "\n")
        }
        if (plan.length > 0)
            socket.flush()
    }

    function handleSubscribeResponse(response) {
        const module = pendingSubscriptions[response.id]

        const remaining = {}
        for (const id in pendingSubscriptions) {
            if (String(id) !== String(response.id))
                remaining[id] = pendingSubscriptions[id]
        }
        pendingSubscriptions = remaining

        const state = Protocol.subscriptionState(response)
        subscriptions = Protocol.withState(subscriptions, module, state)

        // A denied or unavailable subscription means the snapshot is also
        // denied or unavailable.
        if (state === Protocol.Denied || state === Protocol.Unavailable || state === Protocol.Failed) {
            snapshotStates = Protocol.withState(snapshotStates, module, state)
            console.warn("[omnibar-ui] subscription to", module, state + ":", response.error && response.error.message ? response.error.message : "unknown error")
            return
        }

        if (state === Protocol.Subscribed) {
            console.info("[omnibar-ui] subscribed to", module)
            // Fetch the current snapshot after a successful subscription
            // so the UI shows live values without waiting for the next
            // event (ADR-0009: subscribe before fetching avoids a gap).
            if (module === "system_monitor")
                requestSnapshot(module)
        }
    }

    // Request the current snapshot for a module with system.getSnapshot.
    function requestSnapshot(module) {
        if (!socket.connected)
            return

        const requestId = nextRequestId++
        pendingSnapshots = Protocol.withState(pendingSnapshots, requestId, module)
        snapshotStates = Protocol.withState(snapshotStates, module, "pending")

        socket.write(Protocol.buildRequest(requestId, "system.getSnapshot") + "\n")
        socket.flush()
    }

    function handleSnapshotResponse(response) {
        const module = pendingSnapshots[response.id]

        const remaining = {}
        for (const id in pendingSnapshots) {
            if (String(id) !== String(response.id))
                remaining[id] = pendingSnapshots[id]
        }
        pendingSnapshots = remaining

        if (response.error) {
            snapshotStates = Protocol.withState(snapshotStates, module, "failed")
            console.warn("[omnibar-ui] snapshot for", module, "failed:", response.error.message)
            return
        }

        var data = {}
        if (typeof response.result === "object" && response.result !== null) {
            data = response.result
        } else if (typeof response.result === "string") {
            try {
                data = JSON.parse(response.result)
            } catch (error) {
                console.warn("[omnibar-ui] invalid snapshot for", module, error)
                snapshotStates = Protocol.withState(snapshotStates, module, "failed")
                return
            }
        }

        snapshots = Protocol.withState(snapshots, module, data)
        snapshotStates = Protocol.withState(snapshotStates, module, "fresh")
        console.info("[omnibar-ui] snapshot for", module, "received")
        snapshotReceived(module, data)
    }

    function failConnection(reason) {
        handshakeTimer.stop()
        resetSubscriptions()
        pendingHandshakeId = -1
        handshakeComplete = false
        lastError = reason
        status = "unavailable"
        console.warn("[omnibar-ui] backend unavailable:", reason)

        if (socket.connected)
            socket.connected = false

        scheduleRetry()
    }

    function scheduleRetry() {
        // Multiple failure signals can arrive for one connection attempt.
        // Only the first one advances the backoff.
        if (retryTimer.running)
            return

        retryDelayMs = Backoff.delayForAttempt(retryAttempt)
        retryAttempt = Math.min(retryAttempt + 1, 10)
        retryTimer.interval = retryDelayMs
        retryTimer.start()

        console.info("[omnibar-ui] reconnecting in", retryDelayMs, "ms")
    }

    function describeSocketError(error) {
        const errors = [
            "Connection refused",
            "Connection closed by backend",
            "Socket not found",
            "Socket access denied",
            "Socket resource error",
            "Socket timeout",
            "Datagram too large",
            "Connection error",
            "Unsupported socket operation",
            "Unknown socket error",
            "Socket operation error"
        ]

        return error >= 0 && error < errors.length
            ? errors[error]
            : "Unknown socket error"
    }
}
