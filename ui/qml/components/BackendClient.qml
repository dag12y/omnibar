import QtQuick
import Quickshell
import Quickshell.Io
import "backoff.js" as Backoff

// BackendClient owns the Unix socket connection to the Omnibar backend.
//
// It connects to $XDG_RUNTIME_DIR/omnibar/omnibar.sock, performs the
// client.handshake exchange, and reconnects with exponential backoff
// when the backend is unavailable. It never terminates the application
// when the backend is missing.
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

    readonly property bool backendConnected: status === "connected"
    readonly property bool connecting: status === "connecting"
    readonly property bool backendUnavailable: status === "unavailable"
    readonly property bool hasCachedData: backendVersion.length > 0 || modules.length > 0
    readonly property int retryDelaySeconds: Math.round(retryDelayMs / 1000)

    signal handshakeSucceeded()
    signal messageReceived(var message)

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

    function handleConnected() {
        retryTimer.stop()
        handshakeComplete = false
        lastError = ""
        status = "connecting"
        sendHandshake()
    }

    function handleDisconnected() {
        handshakeTimer.stop()
        pendingHandshakeId = -1
        handshakeComplete = false

        if (status !== "unavailable") {
            lastError = "Connection to backend lost"
            status = "unavailable"
        }

        scheduleRetry()
    }

    function handleSocketError(error) {
        handshakeTimer.stop()
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
        handshakeSucceeded()
    }

    function failConnection(reason) {
        handshakeTimer.stop()
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
