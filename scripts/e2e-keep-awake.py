#!/usr/bin/env python3
"""Opt-in live test: briefly inhibit idle/sleep on an active Hyprland session.

Uses isolated Omnibar paths. Normal Go/QML tests never inhibit the host.
"""

import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time


def main():
    root = Path(__file__).resolve().parent.parent
    trial = Path(tempfile.mkdtemp(prefix="omnibar-awake-smoke-"))
    (trial / "run").mkdir(mode=0o700)
    (trial / "config/omnibar").mkdir(parents=True)
    (trial / "config/omnibar/config.toml").write_text(
        (root / "config/config.toml.example").read_text()
    )
    env = dict(os.environ)
    wayland = env["WAYLAND_DISPLAY"]
    if not wayland.startswith("/"):
        wayland = env["XDG_RUNTIME_DIR"] + "/" + wayland
    env.update(
        XDG_RUNTIME_DIR=str(trial / "run"),
        XDG_CONFIG_HOME=str(trial / "config"),
        WAYLAND_DISPLAY=wayland,
        QT_QPA_PLATFORMTHEME="",
        QT_QUICK_CONTROLS_STYLE="Basic",
    )
    backend = ui = conn = stream = None
    qml = str(root / "ui/qml/shell.qml")

    def inhibitors():
        rows = json.loads(subprocess.check_output(
            ["systemd-inhibit", "--list", "--json=short", "--no-pager"], text=True
        ))
        return any(row.get("who") == "Omnibar" and row.get("pid") == backend.pid for row in rows)

    def layers():
        data = json.loads(subprocess.check_output(["hyprctl", "layers", "-j"], text=True))
        return [
            item
            for monitor in data.values()
            for group in monitor.get("levels", {}).values()
            for item in group
        ]

    def surface_mapped():
        return any(item.get("namespace") == "omnibar-keep-awake" and item.get("pid") == ui.pid for item in layers())

    def wait_until(predicate, message):
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            if predicate():
                return
            time.sleep(0.05)
        raise AssertionError(message)

    request_id = 0

    def rpc(method, params=None):
        nonlocal request_id
        request_id += 1
        request = dict(jsonrpc="2.0", id=request_id, method=method, params=params or {})
        stream.write((json.dumps(request) + "\n").encode())
        answer = json.loads(stream.readline())
        assert "error" not in answer, answer
        return answer["result"]

    try:
        with (trial / "backend.log").open("w") as log:
            backend = subprocess.Popen(
                [str(root / "build/omnibar")], env=env, stdout=log, stderr=log
            )
        path = trial / "run/omnibar/omnibar.sock"
        wait_until(path.exists, "backend socket did not appear")
        conn = socket.socket(socket.AF_UNIX)
        conn.settimeout(5)
        conn.connect(str(path))
        stream = conn.makefile("rwb", buffering=0)
        with (trial / "ui.log").open("w") as log:
            ui = subprocess.Popen(
                ["quickshell", "-p", qml], env=env, stdout=log, stderr=log
            )
        assert not rpc("keepAwake.getStatus")["active"]
        assert rpc("keepAwake.start", {"seconds": 15})["active"]
        assert inhibitors()
        print("PASS: real logind inhibitor acquired")
        wait_until(surface_mapped, "Wayland inhibition surface did not map")
        print("PASS: Wayland inhibition surface mapped")
        subprocess.run(
            ["quickshell", "-p", qml, "ipc", "call", "--", "panel", "hide"],
            env=env, check=True,
        )
        assert rpc("keepAwake.getStatus")["active"]
        assert surface_mapped()
        print("PASS: session survives hidden dashboard")
        assert not rpc("keepAwake.stop")["active"]
        wait_until(lambda: not surface_mapped(), "Wayland inhibitor remained after Stop")
        assert not inhibitors()
        print("PASS: Stop releases system and display inhibitors")
        rpc("keepAwake.start", {"seconds": 1})
        wait_until(lambda: not rpc("keepAwake.getStatus")["active"], "session did not expire")
        assert not inhibitors()
        print("PASS: timer expiry releases the session")
        rpc("keepAwake.start", {"seconds": 60})
        backend.send_signal(signal.SIGTERM)
        backend.wait(timeout=5)
        assert backend.returncode == 0
        assert not inhibitors()
        print("PASS: backend shutdown releases the inhibitor")
    finally:
        if stream:
            stream.close()
        if conn:
            conn.close()
        for process in (ui, backend):
            if process and process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
        print("Logs:", trial)
        if (trial / "ui.log").exists():
            print((trial / "ui.log").read_text())


if __name__ == "__main__":
    main()
