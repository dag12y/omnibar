#!/usr/bin/env bash
# End-to-end check of the real Quickshell UI against the real backend.
#
# Verifies, using only an isolated XDG_RUNTIME_DIR/XDG_CONFIG_HOME:
#   1. UI connects, completes the handshake, and subscribes to module events.
#   2. Backend SIGTERM exits 0 and removes the socket.
#   3. UI goes offline and retries with exponential backoff (1000 ms, 2000 ms).
#   4. After the backend restarts, the UI reconnects, re-handshakes, and
#      subscribes again (subscriptions live in backend memory).
#   5. The UI process survives the whole sequence.
#
# OMNIBAR_BIN and OMNIBAR_SHELL_QML override the backend binary and UI entry
# point, which lets the same checks run against an unpacked package.
#
# Requires a running Wayland session (a layer-shell panel is briefly shown) and
# `quickshell`. The user's real Omnibar socket and config are never touched.

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
backend_bin="${OMNIBAR_BIN:-$root/build/omnibar}"
shell_qml="${OMNIBAR_SHELL_QML:-$root/ui/qml/shell.qml}"

if [[ ! -x "$backend_bin" ]]; then
    echo "e2e-ui: backend binary not found at $backend_bin (run 'make build')" >&2
    exit 1
fi
if ! command -v quickshell >/dev/null 2>&1; then
    echo "e2e-ui: quickshell is not installed" >&2
    exit 1
fi
if [[ -z "${WAYLAND_DISPLAY:-}" ]]; then
    echo "e2e-ui: WAYLAND_DISPLAY is not set; a Wayland session is required" >&2
    exit 1
fi

backend_version=$("$backend_bin" --version)
backend_version=${backend_version#omnibar }

# Quickshell needs the compositor socket, which lives in the real runtime dir.
real_runtime="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
wayland_socket="$WAYLAND_DISPLAY"
[[ "$wayland_socket" == /* ]] || wayland_socket="$real_runtime/$wayland_socket"
if [[ ! -S "$wayland_socket" ]]; then
    echo "e2e-ui: Wayland socket $wayland_socket not found" >&2
    exit 1
fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/omnibar-e2e-XXXXXX")"
mkdir -p "$tmp/run" "$tmp/config/omnibar"
chmod 700 "$tmp/run"
cp "$root/config/config.toml.example" "$tmp/config/omnibar/config.toml"
socket="$tmp/run/omnibar/omnibar.sock"
ui_log="$tmp/ui.log"
backend_pid=""
ui_pid=""

cleanup() {
    local status=$?
    [[ -n "$ui_pid" ]] && kill "$ui_pid" 2>/dev/null || true
    [[ -n "$backend_pid" ]] && kill "$backend_pid" 2>/dev/null || true
    wait 2>/dev/null || true
    if [[ $status -ne 0 ]]; then
        echo "--- ui log ---" >&2
        cat "$ui_log" 2>/dev/null >&2 || true
        echo "--- backend logs ---" >&2
        cat "$tmp"/backend-*.log 2>/dev/null >&2 || true
    fi
    rm -rf "$tmp"
    exit "$status"
}
trap cleanup EXIT

fail() {
    echo "e2e-ui: FAIL: $*" >&2
    exit 1
}

pass() {
    echo "e2e-ui: ok: $*"
}

# count PATTERN: how many lines of the UI log match PATTERN.
count() {
    grep -c -- "$1" "$ui_log" 2>/dev/null || true
}

# wait_for_count PATTERN MIN TIMEOUT_SECONDS DESCRIPTION
wait_for_count() {
    local pattern="$1" min="$2" timeout="$3" what="$4"
    local deadline=$((SECONDS + timeout))
    while ((SECONDS < deadline)); do
        if [[ "$(count "$pattern")" -ge "$min" ]]; then
            return 0
        fi
        if ! kill -0 "$ui_pid" 2>/dev/null; then
            fail "UI process exited while waiting for: $what"
        fi
        sleep 0.1
    done
    fail "timed out after ${timeout}s waiting for: $what"
}

start_backend() {
    local n="$1"
    XDG_RUNTIME_DIR="$tmp/run" XDG_CONFIG_HOME="$tmp/config" \
        "$backend_bin" >"$tmp/backend-$n.log" 2>&1 &
    backend_pid=$!
    local deadline=$((SECONDS + 10))
    while ((SECONDS < deadline)); do
        [[ -S "$socket" ]] && return 0
        kill -0 "$backend_pid" 2>/dev/null || fail "backend $n exited during startup"
        sleep 0.05
    done
    fail "backend $n did not create its socket"
}

stop_backend() {
    kill -TERM "$backend_pid"
    local code=0
    wait "$backend_pid" || code=$?
    backend_pid=""
    [[ "$code" -eq 0 ]] || fail "backend exited with code $code after SIGTERM, want 0"
    [[ ! -e "$socket" ]] || fail "socket still exists after graceful shutdown"
}

start_backend 1
pass "backend started"

WAYLAND_DISPLAY="$wayland_socket" XDG_RUNTIME_DIR="$tmp/run" \
    quickshell -p "$shell_qml" >"$ui_log" 2>&1 &
ui_pid=$!

wait_for_count "status: connected" 1 20 "UI to connect"
wait_for_count "handshake complete; backend $backend_version modules: core" 1 5 "UI handshake"
wait_for_count "subscribed to core" 1 5 "UI to subscribe to core events"
wait_for_count "subscribed to system_monitor" 1 5 "UI to subscribe to system_monitor events"
wait_for_count "snapshot for system_monitor received" 1 15 "UI to receive a live system_monitor snapshot"
pass "UI connected, completed handshake, and subscribed; live values arrived"

panel_call() {
    WAYLAND_DISPLAY="$wayland_socket" XDG_RUNTIME_DIR="$tmp/run" quickshell -p "$shell_qml" ipc call -- panel "$1" >/dev/null
}
panel_call hide
wait_for_count "monitoring paused" 1 5 "monitoring to pause while hidden"
panel_call show
wait_for_count "snapshot for system_monitor received" 2 10 "fresh metrics after reopening"
pass "panel hide/show pauses monitoring and requests a fresh snapshot"

stop_backend
pass "backend SIGTERM exit 0 and socket removed"

wait_for_count "status: unavailable" 1 10 "UI to report backend unavailable"
wait_for_count "snapshots marked stale" 1 10 "UI to mark snapshots stale"
wait_for_count "reconnecting in 1000 ms" 1 10 "first backoff (1000 ms)"
wait_for_count "reconnecting in 2000 ms" 1 10 "second backoff (2000 ms)"
pass "UI offline with exponential backoff (1000 ms, 2000 ms) and stale snapshots"

start_backend 2
wait_for_count "status: connected" 2 30 "UI to reconnect after backend restart"
wait_for_count "handshake complete; backend $backend_version modules: core" 2 5 "UI re-handshake"
wait_for_count "subscribed to core" 2 5 "UI to re-subscribe after backend restart"
pass "UI reconnected, re-handshook, and re-subscribed after backend restart"

[[ "$(count "\[omnibar-ui\] subscription to")" -eq 0 ]] ||
    fail "UI reported a failed subscription: $(grep -- "subscription to" "$ui_log" | head -3)"

kill -0 "$ui_pid" 2>/dev/null || fail "UI process died during the test"
stop_backend
pass "final backend shutdown clean"

echo "e2e-ui: all checks passed"
