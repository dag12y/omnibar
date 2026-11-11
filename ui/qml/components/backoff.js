.pragma library

var minDelayMs = 1000
var maxDelayMs = 30000

// delayForAttempt returns the reconnect delay for a zero-based failure
// count: 1s, 2s, 4s, 8s, 16s, 30s, 30s, ...
function delayForAttempt(attempt) {
    if (!isFinite(attempt) || attempt <= 0)
        return minDelayMs

    return Math.min(maxDelayMs, minDelayMs * Math.pow(2, attempt))
}
