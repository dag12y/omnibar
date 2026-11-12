#!/usr/bin/env python3
"""Test session clipboard history in a separate nested Wayland compositor.

Requires Hyprland, wl-clipboard, Quickshell and an active Wayland desktop.
Only synthetic text is copied in the private compositor. --capture saves its UI.
"""
import argparse
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--capture", action="store_true")
    parser.add_argument("--shelf", action="store_true", help="also verify the File shelf with synthetic files")
    parser.add_argument("--windows", action="store_true", help="verify window switching inside the private compositor")
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    # Keep Hyprland's instance Unix-socket path below Linux's length limit.
    trial = Path(tempfile.mkdtemp(prefix="obc-"))
    (trial / "run").mkdir(mode=0o700)
    (trial / "config/omnibar").mkdir(parents=True)
    config = (root / "config/config.toml.example").read_text()
    config = config.replace('"clipboard.read" = false', '"clipboard.read" = true')
    config = config.replace('"clipboard.write" = false', '"clipboard.write" = true')
    if args.shelf:
        config = config.replace('"filesystem.access" = false', '"filesystem.access" = true')
    if args.windows:
        config = config.replace('"window.manage" = false', '"window.manage" = true')
    (trial / "config/omnibar/config.toml").write_text(config)
    (trial / "hyprland.conf").write_text(
        "monitor = , 900x800, auto, 1\n"
        "misc {\n disable_hyprland_logo = true\n disable_splash_rendering = true\n}\n"
        "xwayland {\n enabled = false\n}\n"
    )
    host_wayland = os.environ["WAYLAND_DISPLAY"]
    if not host_wayland.startswith("/"):
        host_wayland = os.environ["XDG_RUNTIME_DIR"] + "/" + host_wayland
    env = dict(os.environ)
    env.update(
        XDG_RUNTIME_DIR=str(trial / "run"), XDG_CONFIG_HOME=str(trial / "config"),
        XDG_STATE_HOME=str(trial / "state"), XDG_DATA_HOME=str(trial / "data"), WAYLAND_DISPLAY=host_wayland,
        AQ_DRM_DEVICES="/dev/null", HYPRLAND_NO_SD_VARS="1", HYPRLAND_NO_SD_NOTIFY="1",
        QT_QPA_PLATFORM="wayland", QT_QPA_PLATFORMTHEME="", QT_QUICK_CONTROLS_STYLE="Basic",
    )
    if args.shelf:
        # Record opener arguments without launching a file manager on the host.
        (trial / "bin").mkdir()
        opener = trial / "bin/xdg-open"
        opener.write_text('#!/usr/bin/python3\nimport json, os, sys\nfrom pathlib import Path\nPath(os.environ["OMNIBAR_TEST_OPEN_LOG"]).write_text(json.dumps(sys.argv[1:]))\n')
        opener.chmod(0o700)
        env["PATH"] = str(trial / "bin") + ":" + env["PATH"]
        env["OMNIBAR_TEST_OPEN_LOG"] = str(trial / "opened.json")
    processes = []
    conn = stream = None
    qml = str(root / "ui/qml/shell.qml")

    def launch(label, *command):
        with (trial / (label + ".log")).open("a") as log:
            proc = subprocess.Popen(command, env=env, stdout=log, stderr=log)
        processes.append(proc)
        return proc

    def stop(proc):
        if proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait()

    def wait_until(predicate, message):
        end = time.monotonic() + 10
        while time.monotonic() < end:
            if predicate():
                return
            time.sleep(0.05)
        raise AssertionError(message)

    def copy(text, sensitive=False):
        command = ["wl-copy", "--type", "text/plain;charset=utf-8"]
        if sensitive:
            command.append("--sensitive")
        subprocess.run(command, input=text, text=True, env=env, check=True, timeout=5)

    def paste():
        return subprocess.check_output(["wl-paste", "--no-newline"], env=env, text=True, timeout=5)

    request_id = 0

    def rpc(method, params=None):
        nonlocal request_id
        request_id += 1
        stream.write((json.dumps(dict(jsonrpc="2.0", id=request_id, method=method, params=params or {})) + "\n").encode())
        response = json.loads(stream.readline())
        assert "error" not in response, response
        return response["result"]

    def rpc_error(method, params):
        nonlocal request_id
        request_id += 1
        stream.write((json.dumps(dict(jsonrpc="2.0", id=request_id, method=method, params=params)) + "\n").encode())
        response = json.loads(stream.readline())
        assert "error" in response, response
        return response["error"]

    def snapshot():
        return rpc("clipboard.getSnapshot")

    def newest_is(field, value):
        entries = snapshot()["entries"]
        return bool(entries) and entries[0][field] == value

    try:
        compositor = launch("hyprland", "Hyprland", "--config", str(trial / "hyprland.conf"))
        wait_until(lambda: any(path.is_socket() for path in (trial / "run").glob("wayland-*")),
                   "nested Wayland compositor did not start")
        assert compositor.poll() is None
        env["WAYLAND_DISPLAY"] = str(next(path for path in (trial / "run").glob("wayland-*") if path.is_socket()))
        wait_until(lambda: bool(list((trial / "run/hypr").glob("*/hyprland.lock"))), "private Hyprland instance was not registered")
        instance_lock = next((trial / "run/hypr").glob("*/hyprland.lock"))
        env["HYPRLAND_INSTANCE_SIGNATURE"] = instance_lock.parent.name
        backend = launch("backend", str(root / "build/omnibar"))
        path = trial / "run/omnibar/omnibar.sock"
        wait_until(path.exists, "backend did not start")
        conn = socket.socket(socket.AF_UNIX)
        conn.settimeout(8)
        conn.connect(str(path))
        stream = conn.makefile("rwb", buffering=0)
        assert not snapshot()["collecting"] and not snapshot()["entries"]
        copy("before collection")
        assert not snapshot()["entries"]
        rpc("clipboard.setCollecting", dict(collecting=True))
        first = "First copied text\nWith an exact newline."
        copy(first)
        wait_until(lambda: any(entry["text"] == first for entry in snapshot()["entries"]), "text not collected")
        original_id = next(entry["id"] for entry in snapshot()["entries"] if entry["text"] == first)
        copy("Second copied text")
        wait_until(lambda: newest_is("text", "Second copied text"), "second copy not collected")
        copy(first)
        wait_until(lambda: newest_is("id", original_id), "duplicate did not keep stable ID")
        assert sum(entry["text"] == first for entry in snapshot()["entries"]) == 1
        copy("Sensitive fixture", sensitive=True)
        time.sleep(0.2)
        assert not any(entry["text"] == "Sensitive fixture" for entry in snapshot()["entries"])
        rpc("clipboard.copy", dict(id=original_id))
        assert paste() == first
        print("PASS: explicit collection, exact text, deduplication, sensitive hint, and copy-back")
        rpc("clipboard.setCollecting", dict(collecting=False))
        copy("While paused")
        time.sleep(0.15)
        assert not any(entry["text"] == "While paused" for entry in snapshot()["entries"])
        rpc("clipboard.delete", dict(id=original_id))
        assert not any(entry["id"] == original_id for entry in snapshot()["entries"])
        rpc("clipboard.clear")
        assert not snapshot()["entries"] and paste() == "While paused"
        print("PASS: Pause, delete, and Clear leave the current clipboard alone")
        rpc("clipboard.setCollecting", dict(collecting=True))
        copy("Omnibar clipboard history\nText stays in memory for this session.")
        wait_until(lambda: any("Omnibar clipboard history" in entry["text"] for entry in snapshot()["entries"]), "resume did not collect")
        saved_text = "Omnibar clipboard history\nText stays in memory for this session."
        saved_id = next(entry["id"] for entry in snapshot()["entries"] if entry["text"] == saved_text)
        rpc("clipboard.setFavorite", dict(id=saved_id, favorite=True))
        assert snapshot()["entries"][0]["favorite"]
        rpc("clipboard.clear")
        assert len(snapshot()["entries"]) == 1
        saved_file = trial / "data/omnibar/clipboard-favorites.json"
        assert saved_file.stat().st_mode & 0o777 == 0o600
        assert saved_file.parent.stat().st_mode & 0o777 == 0o700
        print("PASS: explicit favorite save, private file permissions, and Clear recent preserves favorites")
        rpc("clipboard.setCollecting", dict(collecting=False))
        favorite_bytes = saved_file.read_bytes()
        source = next(entry for entry in snapshot()["entries"] if entry["id"] == saved_id)
        for operation, expected in [
            ("trim", saved_text), ("lowercase", saved_text.lower()),
            ("uppercase", saved_text.upper()), ("remove_line_breaks", saved_text.replace("\n", " ")),
        ]:
            rpc("clipboard.transformCopy", dict(id=saved_id, operation=operation))
            assert paste() == expected
            assert next(entry for entry in snapshot()["entries"] if entry["id"] == saved_id) == source
            assert saved_file.read_bytes() == favorite_bytes
        rpc("clipboard.setCollecting", dict(collecting=True))
        rpc("clipboard.transformCopy", dict(id=saved_id, operation="uppercase"))
        wait_until(lambda: any(entry["text"] == saved_text.upper() for entry in snapshot()["entries"]), "transformed copy did not enter recent history")
        assert next(entry for entry in snapshot()["entries"] if entry["id"] == saved_id) == source
        copy(" \t\n ")
        wait_until(lambda: any(entry["text"] == " \t\n " for entry in snapshot()["entries"]), "whitespace fixture not collected")
        rpc("clipboard.setCollecting", dict(collecting=False))
        empty_id = next(entry["id"] for entry in snapshot()["entries"] if entry["text"] == " \t\n ")
        before_empty = paste()
        assert rpc_error("clipboard.transformCopy", dict(id=empty_id, operation="trim"))["code"] == -32603
        assert paste() == before_empty
        assert rpc_error("clipboard.transformCopy", dict(id=saved_id, operation="unknown"))["code"] == -32602
        assert paste() == before_empty
        rpc("clipboard.clear")
        assert saved_file.read_bytes() == favorite_bytes
        rpc("clipboard.setCollecting", dict(collecting=True))
        print("PASS: all real transform copies, unchanged source/favorite file, active collection, and empty/invalid result rejection")
        ui = launch("ui", "quickshell", "-p", qml)
        wait_until(lambda: "handshake complete" in (trial / "ui.log").read_text(), "UI did not connect")
        subprocess.run(["quickshell", "-p", qml, "ipc", "call", "--", "panel", "showClipboard"], env=env, check=True, stdout=subprocess.DEVNULL)
        time.sleep(0.3)
        subprocess.run(["quickshell", "-p", qml, "ipc", "call", "--", "panel", "hide"], env=env, check=True, stdout=subprocess.DEVNULL)
        copy("Copied while the panel is hidden")
        wait_until(lambda: any(entry["text"] == "Copied while the panel is hidden" for entry in snapshot()["entries"]), "hidden UI stopped collection")
        subprocess.run(["quickshell", "-p", qml, "ipc", "call", "--", "panel", "show"], env=env, check=True, stdout=subprocess.DEVNULL)
        print("PASS: real Clipboard page and hidden-panel collection")
        if args.shelf:
            fixtures = trial / "fixtures"
            fixtures.mkdir()
            sample = fixtures / "Example # note λ.txt"
            sample.write_text("Original synthetic text stays here.\n")
            added = rpc("shelf.add", dict(paths=[sample.as_uri(), str(fixtures)]))
            assert len(added["entries"]) == 2 and added["entries"][0]["path"] == str(sample)
            shelf_id = added["entries"][0]["id"]
            assert rpc("shelf.add", dict(paths=[str(sample)]))["entries"][0]["id"] == shelf_id
            rpc("shelf.copyPath", dict(id=shelf_id))
            assert paste() == str(sample)
            rpc("shelf.openFolder", dict(id=shelf_id))
            assert json.loads((trial / "opened.json").read_text()) == [str(fixtures)]
            sample.unlink()
            assert not rpc("shelf.getSnapshot")["entries"][0]["available"]
            sample.write_text("Original synthetic text stays here.\n")
            assert rpc("shelf.getSnapshot")["entries"][0]["available"]
            subprocess.run(["quickshell", "-p", qml, "ipc", "call", "--", "panel", "showFiles"], env=env, check=True, stdout=subprocess.DEVNULL)
            print("PASS: File shelf URL batch/dedup, real copy-path, recorded folder opener, unavailable/recovery, and real Files page")
        if args.windows:
            fixture_qml = trial / "windows.qml"
            fixture_qml.write_text('import QtQuick\nimport QtQuick.Window\nimport Quickshell\nShellRoot { Window { visible:true; width:320; height:180; title:"Omnibar fixture Alpha"; color:"#263342" } Window { visible:true; width:320; height:180; title:"Omnibar fixture Beta"; color:"#344226" } }\n')
            fixture_app = launch("window-fixture", "quickshell", "-p", str(fixture_qml))
            def compositor(query):
                return json.loads(subprocess.check_output(["hyprctl", "-j", query], env=env, text=True))
            wait_until(lambda: sum(item.get("title", "").startswith("Omnibar fixture ") for item in compositor("clients")) == 2, "fixture windows did not map")
            clients = compositor("clients")
            address_a = next(item["address"] for item in clients if item.get("title") == "Omnibar fixture Alpha")
            address_b = next(item["address"] for item in clients if item.get("title") == "Omnibar fixture Beta")
            subprocess.run(["hyprctl", "dispatch", "movetoworkspacesilent", "2,address:" + address_b], env=env, check=True, stdout=subprocess.DEVNULL)
            listed = rpc("windows.getSnapshot")
            assert any(item["address"] == address_b and item["workspace"] == "2" for item in listed["windows"])
            subprocess.run(["quickshell", "-p", qml, "ipc", "call", "--", "panel", "showWindows"], env=env, check=True, stdout=subprocess.DEVNULL)
            for address in [address_a, address_b]:
                switched = rpc("windows.focus", dict(address=address))
                assert any(item["address"] == address and item["active"] for item in switched["windows"])
                assert compositor("activewindow")["address"] == address
            assert compositor("activeworkspace")["id"] == 2
            assert rpc_error("windows.focus", dict(address="0x1;exec invalid"))["code"] == -32602
            stop(fixture_app)
            wait_until(lambda: not any(item.get("title", "").startswith("Omnibar fixture ") for item in compositor("clients")), "closed fixtures still mapped")
            assert rpc_error("windows.focus", dict(address=address_a))["code"] == -32603
            assert not rpc("windows.getSnapshot")["windows"]
            print("PASS: private running-window list, confirmed focus across workspaces, real Windows page, invalid/stale targets, and empty desktop")
        if args.capture:
            time.sleep(1.2)
            instances = list((trial / "run/hypr").glob("*/hyprland.lock"))
            assert instances, "nested compositor IPC is unavailable"
            local_env = dict(env, HYPRLAND_INSTANCE_SIGNATURE=instances[0].parent.name)
            layers = json.loads(subprocess.check_output(["hyprctl", "layers", "-j"], env=local_env, text=True))
            panel = next(item for monitor in layers.values() for group in monitor.get("levels", {}).values()
                         for item in group if item.get("namespace") == "omnibar-panel")
            geometry = f"{panel['x']},{panel['y']} {panel['w']}x{panel['h']}"
            subprocess.run(["grim", "-g", geometry, str(trial / ("windows.png" if args.windows else "files.png" if args.shelf else "clipboard.png"))], env=env, check=True)
        if args.shelf:
            rpc("shelf.clear")
            assert sample.read_text() == "Original synthetic text stays here.\n"
            rpc("shelf.add", dict(paths=[str(sample)]))
            # Keep the clipboard assertion for the existing history lifecycle test.
            copy("Copied while the panel is hidden")
            print("PASS: Clear shelf preserves originals; session shelf seeded for restart")
        backend.send_signal(signal.SIGTERM)
        backend.wait(timeout=5)
        stream.close(); stream = None
        conn.close(); conn = None
        assert paste() == "Copied while the panel is hidden"
        backend = launch("backend", str(root / "build/omnibar"))
        wait_until(path.exists, "backend did not restart")
        conn = socket.socket(socket.AF_UNIX); conn.settimeout(8); conn.connect(str(path))
        stream = conn.makefile("rwb", buffering=0)
        if args.shelf:
            assert not rpc("shelf.getSnapshot")["entries"]
            assert sample.exists()
            print("PASS: backend restart forgets shelf references and preserves original files")
        restored = snapshot()
        assert not restored["collecting"] and len(restored["entries"]) == 1
        assert restored["entries"][0]["text"] == saved_text and restored["entries"][0]["favorite"]
        assert "Copied while the panel is hidden" not in saved_file.read_text()
        rpc("clipboard.copy", dict(id=restored["entries"][0]["id"]))
        assert paste() == saved_text
        rpc("clipboard.delete", dict(id=restored["entries"][0]["id"]))
        assert not snapshot()["entries"] and not json.loads(saved_file.read_text())["entries"]
        print("PASS: restart restores only favorites, stays paused, and saved text can be copied and removed")
    finally:
        if stream:
            stream.close()
        if conn:
            conn.close()
        for proc in reversed(processes):
            stop(proc)
        print("Artifacts:", trial)
        if (trial / "ui.log").exists():
            print((trial / "ui.log").read_text())


if __name__ == "__main__":
    main()
