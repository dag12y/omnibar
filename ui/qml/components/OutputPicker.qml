import QtQuick

// Playback-compatible wrapper around the shared microphone/output picker.
DevicePicker {
    id: root
    property alias outputs: root.devices
    property alias selectedOutput: root.selectedDevice
    signal outputChosen(string output)
    emptyText: "No outputs connected"
    missingText: "Output unavailable"
    Accessible.name: "Playback output"
    onDeviceChosen: device => outputChosen(device)
}
