#!/bin/sh
# Development equivalent of the installed omnibar-toggle command.
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
qml="$root/ui/qml/shell.qml"
if quickshell -p "$qml" ipc call -- panel toggle 2>/dev/null; then
    exit 0
fi
exec quickshell -n -d -p "$qml"
