#!/usr/bin/env python3
"""Exercise Omnibar against isolated PipeWire services and virtual outputs.

Requires an active Hyprland desktop, Python 3, PipeWire, pipewire-pulse,
WirePlumber, pactl, pacat, and Quickshell. --capture optionally saves a panel shot.
No host outputs, playback streams, or audio services are changed.
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
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    trial = Path(tempfile.mkdtemp(prefix="omnibar-audio-e2e-"))
    (trial / "config/omnibar").mkdir(parents=True)
    (trial / "run").mkdir(mode=0o700)
    (trial / "config/omnibar/config.toml").write_text(
        (root / "config/config.toml.example").read_text()
    )
    wayland = os.environ["WAYLAND_DISPLAY"]
    if not wayland.startswith("/"):
        wayland = os.environ["XDG_RUNTIME_DIR"] + "/" + wayland
    env = dict(os.environ)
    env.update(
        XDG_RUNTIME_DIR=str(trial / "run"),
        XDG_CONFIG_HOME=str(trial / "config"),
        XDG_STATE_HOME=str(trial / "state"),
        PIPEWIRE_RUNTIME_DIR=str(trial / "run"),
        PIPEWIRE_REMOTE="pipewire-0",
        PIPEWIRE_CONFIG_DIR="/usr/share/pipewire",
        WIREPLUMBER_CONFIG_DIR="/usr/share/wireplumber",
        PULSE_SERVER="unix:" + str(trial / "run/pulse/native"),
        PULSE_RUNTIME_PATH=str(trial / "run/pulse"),
        WAYLAND_DISPLAY=wayland,
        QT_QPA_PLATFORMTHEME="",
        QT_QUICK_CONTROLS_STYLE="Basic",
    )
    processes = []
    conn = stream = None
    qml = str(root / "ui/qml/shell.qml")

    def launch(label, *command):
        with (trial / (label + ".log")).open("a") as log:
            process = subprocess.Popen(command, env=env, stdout=log, stderr=log)
        processes.append(process)
        return process

    def run(*command):
        return subprocess.check_output(command, env=env, text=True, timeout=8)

    def wait_until(predicate, message):
        deadline = time.monotonic() + 12
        while time.monotonic() < deadline:
            try:
                if predicate():
                    return
            except (ConnectionError, OSError, subprocess.SubprocessError):
                pass
            time.sleep(0.05)
        raise AssertionError(message)

    def ui_log():
        return (trial / "ui.log").read_text()

    def panel_call(action, *arguments):
        run("quickshell", "-p", qml, "ipc", "call", "--", "panel", action, *arguments)

    def stop(process):
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()

    request_id = 0

    def rpc(method, params=None, expect_error=False):
        nonlocal request_id
        request_id += 1
        request = dict(jsonrpc="2.0", id=request_id, method=method, params=params or {})
        stream.write((json.dumps(request) + "\n").encode())
        answer = json.loads(stream.readline())
        if expect_error:
            assert "error" in answer, answer
            return answer["error"]
        assert "error" not in answer, answer
        return answer["result"]

    def snapshot():
        return rpc("audio.getSnapshot")

    try:
        core = launch("pipewire", "pipewire")
        wait_until(lambda: (trial / "run/pipewire-0").exists(), "private PipeWire did not start")
        pulse = launch("pulse", "pipewire-pulse")
        wait_until(lambda: (trial / "run/pulse/native").exists(), "private pulse service did not start")
        # This profile provides default-node and linking policy without hardware
        # monitors. It never enumerates ALSA, Bluetooth, or camera devices.
        policy = launch("policy", "wireplumber", "--profile=policy")
        time.sleep(0.5)
        run("pactl", "load-module", "module-null-sink", "sink_name=omnibar_one",
            "sink_properties=device.description=Test_Speakers")
        second = run("pactl", "load-module", "module-null-sink", "sink_name=omnibar_two",
                     "sink_properties=device.description=Test_Headphones").strip()
        microphone_modules = {}
        for suffix, description in (("a", "Desk_Microphone"), ("b", "Headset_Microphone")):
            fifo = trial / ("mic-" + suffix + ".fifo")
            os.mkfifo(fifo)
            microphone_modules[suffix] = run("pactl", "load-module", "module-pipe-source",
                "file=" + str(fifo), "source_name=omnibar_mic_" + suffix,
                "source_properties=device.description=" + description,
                "format=s16le", "rate=48000", "channels=1").strip()
        playback = launch("playback", "pacat", "--playback", "--device=omnibar_one",
                          "--client-name=Omnibar Test", "/dev/zero")
        backend = launch("backend", str(root / "build/omnibar"))
        path = trial / "run/omnibar/omnibar.sock"
        wait_until(path.exists, "backend socket did not appear")
        conn = socket.socket(socket.AF_UNIX)
        conn.settimeout(8)
        conn.connect(str(path))
        stream = conn.makefile("rwb", buffering=0)
        wait_until(lambda: len(snapshot()["streams"]) == 1, "test playback did not appear")
        snap = snapshot()
        assert len(snap["outputs"]) == 2
        key = snap["streams"][0]["key"]
        print("PASS: isolated outputs and playback stream discovered")
        assert rpc("audio.setVolume", dict(key=key, volume=37))["streams"][0]["volume"] == 37
        assert rpc("audio.setMute", dict(key=key, muted=True))["streams"][0]["muted"]
        assert not rpc("audio.setMute", dict(key=key, muted=False))["streams"][0]["muted"]
        assert rpc("audio.setDefaultOutput", dict(output="omnibar_two"))["default_output"] == "omnibar_two"
        print("PASS: volume, mute/unmute, and default output selection")
        control = launch("control-playback", "pacat", "--playback", "--device=omnibar_two",
                         "--client-name=Omnibar Control", "/dev/zero")
        wait_until(lambda: len(snapshot()["streams"]) == 2, "control stream did not appear")
        wait_until(lambda: all(item["output_id"] in [output["id"] for output in snapshot()["outputs"]]
                               for item in snapshot()["streams"]), "streams did not connect to outputs")
        before = snapshot()
        other = next(item for item in before["streams"] if item["key"] != key)
        destination = next(output for output in before["outputs"] if output["name"] == "omnibar_one")
        rpc("audio.setMute", dict(key=key, muted=True))
        moved = rpc("audio.moveStream", dict(key=key, output=destination["name"]))
        routed = next(item for item in moved["streams"] if item["key"] == key)
        unaffected = next(item for item in moved["streams"] if item["key"] == other["key"])
        assert routed["output_id"] == destination["id"], moved
        assert routed["volume"] == 37 and routed["muted"], routed
        assert moved["default_output"] == before["default_output"]
        for field in ("output_id", "volume", "muted"):
            assert unaffected[field] == other[field], (field, other, unaffected)
        rpc("audio.setMute", dict(key=key, muted=False))
        print("PASS: stream routing preserves default output, other playback, volume, and mute")
        wait_until(lambda: len([input for input in snapshot()["inputs"] if not input["monitor"]]) == 2,
                   "virtual microphones did not appear")
        before_mics = snapshot()
        microphone_a = next(input for input in before_mics["inputs"] if input["name"] == "omnibar_mic_a")
        microphone_b = next(input for input in before_mics["inputs"] if input["name"] == "omnibar_mic_b")
        rpc("audio.setDefaultInput", dict(input="omnibar_mic_a"))
        changed = rpc("audio.setInputVolume", dict(input="omnibar_mic_a", volume=43))
        assert next(input for input in changed["inputs"] if input["name"] == "omnibar_mic_a")["volume"] == 43
        muted = rpc("audio.setInputMute", dict(input="omnibar_mic_a", muted=True))
        assert next(input for input in muted["inputs"] if input["name"] == "omnibar_mic_a")["muted"]
        switched = rpc("audio.setDefaultInput", dict(input="omnibar_mic_b"))
        assert switched["default_input"] == "omnibar_mic_b"
        assert switched["default_output"] == before_mics["default_output"]
        assert next(input for input in switched["inputs"] if input["name"] == "omnibar_mic_b")["volume"] == microphone_b["volume"]
        assert next(input for input in switched["inputs"] if input["name"] == "omnibar_mic_a")["muted"]
        assert switched["streams"] == before_mics["streams"]
        rpc("audio.setInputMute", dict(input="omnibar_mic_a", muted=False))
        rpc("audio.setInputVolume", dict(input="omnibar_mic_b", volume=68))
        recorder = launch("recorder", "pacat", "--record", "--device=omnibar_mic_a",
                          "--client-name=Omnibar Recorder", "/dev/null")
        wait_until(lambda: any(item["name"] == "Omnibar Recorder" and item["input_id"] == microphone_a["id"]
                               for item in snapshot()["recordings"]), "recording app/source was not discovered")
        monitor = next(input for input in snapshot()["inputs"] if input["monitor"])
        error = rpc("audio.setInputMute", dict(input=monitor["name"], muted=True), expect_error=True)
        assert "not an output monitor" in error["message"], error
        print("PASS: microphone gain/mute/default, capture metadata, and monitor exclusion")
        ui = launch("ui", "quickshell", "-p", qml)
        wait_until(lambda: "handshake complete" in ui_log(), "UI did not handshake")
        panel_call("selectPage", "audio")
        wait_until(lambda: "audio snapshot received" in ui_log(), "Audio page did not receive state")
        print("PASS: real Audio page connected")
        before_view = snapshot()
        panel_call("showMicrophone")
        wait_until(lambda: "microphone controls loaded" in ui_log(), "microphone UI did not load")
        after_view = snapshot()
        assert after_view["inputs"] == before_view["inputs"]
        assert after_view["default_input"] == before_view["default_input"]
        assert {item["id"] for item in after_view["recordings"]} == {item["id"] for item in before_view["recordings"]}
        print("PASS: Microphone view opens without changing input state or recording")
        if args.capture:
            time.sleep(0.3)
            layers = json.loads(subprocess.check_output(["hyprctl", "layers", "-j"], text=True))
            panel = next(item for monitor in layers.values()
                         for group in monitor.get("levels", {}).values() for item in group
                         if item.get("pid") == ui.pid and item.get("namespace") == "omnibar-panel")
            geometry = f"{panel['x']},{panel['y']} {panel['w']}x{panel['h']}"
            subprocess.run(["grim", "-g", geometry, str(trial / "audio.png")], check=True)
        stop(recorder)
        wait_until(lambda: not any(item["name"] == "Omnibar Recorder" for item in snapshot()["recordings"]),
                   "ended recording stream stayed listed")
        run("pactl", "unload-module", microphone_modules["b"])
        wait_until(lambda: not any(input["name"] == "omnibar_mic_b" for input in snapshot()["inputs"]),
                   "disconnected microphone stayed listed")
        error = rpc("audio.setInputVolume", dict(input="omnibar_mic_b", volume=50), expect_error=True)
        assert "no longer available" in error["message"], error
        print("PASS: microphone and recording-app removal; stale input rejected")
        run("pactl", "unload-module", second)
        wait_until(lambda: len(snapshot()["outputs"]) == 1, "removed output stayed listed")
        error = rpc("audio.moveStream", dict(key=key, output="omnibar_two"), expect_error=True)
        assert "no longer available" in error["message"], error
        stop(playback)
        stop(control)
        wait_until(lambda: not snapshot()["streams"], "ended stream stayed listed")
        error = rpc("audio.moveStream", dict(key=key, output="omnibar_one"), expect_error=True)
        assert "stream ended or changed" in error["message"], error
        print("PASS: output/stream removal and stale route rejection")
        ready_count = ui_log().count("audio state: ready")
        stop(pulse)
        wait_until(lambda: "audio state: unavailable" in ui_log(), "UI did not notice service loss")
        assert not snapshot()["available"]
        pulse = launch("pulse", "pipewire-pulse")
        wait_until(lambda: ui_log().count("audio state: ready") > ready_count,
                   "audio watcher did not recover after pulse service restart")
        print("PASS: service loss and automatic watcher recovery")
        ready_count = ui_log().count("audio state: ready")
        stop(pulse)
        stop(policy)
        stop(core)
        core = launch("pipewire", "pipewire")
        wait_until(lambda: (trial / "run/pipewire-0").exists(), "core did not restart")
        pulse = launch("pulse", "pipewire-pulse")
        policy = launch("policy", "wireplumber", "--profile=policy")
        wait_until(lambda: ui_log().count("audio state: ready") > ready_count,
                   "audio watcher did not recover after full core restart")
        print("PASS: private PipeWire core restart recovery")
        pause_count = ui_log().count("audio paused")
        panel_call("hide")
        wait_until(lambda: ui_log().count("audio paused") > pause_count, "hidden page retained demand")
        panel_call("show")
        wait_until(lambda: ui_log().count("audio snapshot received") >= 2, "Audio page did not reopen")
        print("PASS: hide/show audio demand")
        backend.send_signal(signal.SIGTERM)
        backend.wait(timeout=5)
        assert backend.returncode == 0
    finally:
        if stream:
            stream.close()
        if conn:
            conn.close()
        for process in reversed(processes):
            stop(process)
        print("Artifacts:", trial)
        if (trial / "ui.log").exists():
            print(ui_log())


if __name__ == "__main__":
    main()
