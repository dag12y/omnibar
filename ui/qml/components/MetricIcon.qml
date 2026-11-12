import QtQuick
import QtQuick.Shapes

// Monochrome SF Symbol-style stroke icons, drawn with QtQuick.Shapes.
// 1.5px stroke width, round caps, currentColor. No external font or icon path.
// Names: cpu, memory, swap, disk, network, battery, retry, refresh.
Item {
    id: root

    property string name: "cpu"
    property real size: 16
    property color color: "#dfe4ec"

    readonly property real lw: 1.5

    width: size
    height: size

    // ----- cpu: processor chip outline -----
    Shape {
        visible: root.name === "cpu"
        width: root.size
        height: root.size
        ShapePath {
            strokeColor: root.color
            strokeWidth: root.lw
            capStyle: ShapePath.RoundCap
            joinStyle: ShapePath.RoundJoin
            startX: root.size * 0.14; startY: root.size * 0.14
            PathLine { x: root.size * 0.86; y: root.size * 0.14 }
            PathLine { x: root.size * 0.86; y: root.size * 0.86 }
            PathLine { x: root.size * 0.14; y: root.size * 0.86 }
            PathLine { x: root.size * 0.14; y: root.size * 0.14 }
            PathLine { x: root.size * 0.50; y: root.size * 0.50 }
            PathMove { x: root.size * 0.30; y: root.size * 0.30 }
            PathLine { x: root.size * 0.70; y: root.size * 0.30 }
            PathMove { x: root.size * 0.30; y: root.size * 0.70 }
            PathLine { x: root.size * 0.70; y: root.size * 0.70 }
        }
    }

    // ----- memory: stacked bar chip -----
    Shape {
        visible: root.name === "memory"
        width: root.size
        height: root.size
        ShapePath {
            strokeColor: root.color
            strokeWidth: root.lw
            capStyle: ShapePath.RoundCap
            joinStyle: ShapePath.RoundJoin
            startX: root.size * 0.22; startY: root.size * 0.22
            PathLine { x: root.size * 0.78; y: root.size * 0.22 }
            PathLine { x: root.size * 0.78; y: root.size * 0.86 }
            PathLine { x: root.size * 0.22; y: root.size * 0.86 }
            PathLine { x: root.size * 0.22; y: root.size * 0.22 }
            PathMove { x: root.size * 0.30; y: root.size * 0.32 }
            PathLine { x: root.size * 0.70; y: root.size * 0.32 }
            PathMove { x: root.size * 0.38; y: root.size * 0.48 }
            PathLine { x: root.size * 0.62; y: root.size * 0.48 }
            PathMove { x: root.size * 0.46; y: root.size * 0.64 }
            PathLine { x: root.size * 0.54; y: root.size * 0.64 }
        }
    }

    // ----- swap: two opposing arrows -----
    Shape {
        visible: root.name === "swap"
        width: root.size
        height: root.size
        ShapePath {
            strokeColor: root.color
            strokeWidth: root.lw
            capStyle: ShapePath.RoundCap
            joinStyle: ShapePath.RoundJoin
            startX: root.size * 0.22; startY: root.size * 0.30
            PathLine { x: root.size * 0.50; y: root.size * 0.20 }
            PathLine { x: root.size * 0.78; y: root.size * 0.30 }
            PathMove { x: root.size * 0.50; y: root.size * 0.20 }
            PathLine { x: root.size * 0.50; y: root.size * 0.80 }
            PathMove { x: root.size * 0.78; y: root.size * 0.70 }
            PathLine { x: root.size * 0.50; y: root.size * 0.80 }
            PathLine { x: root.size * 0.22; y: root.size * 0.70 }
            PathMove { x: root.size * 0.50; y: root.size * 0.80 }
        }
    }

    // ----- disk: drive platter + base -----
    Shape {
        visible: root.name === "disk"
        width: root.size
        height: root.size
        ShapePath {
            strokeColor: root.color
            strokeWidth: root.lw
            capStyle: ShapePath.RoundCap
            joinStyle: ShapePath.RoundJoin
            startX: root.size * 0.18; startY: root.size * 0.42
            PathArc { x: root.size * 0.82; y: root.size * 0.42; radiusX: root.size * 0.32; radiusY: root.size * 0.18 }
            PathArc { x: root.size * 0.18; y: root.size * 0.42; radiusX: root.size * 0.32; radiusY: root.size * 0.18 }
            PathMove { x: root.size * 0.18; y: root.size * 0.42 }
            PathLine { x: root.size * 0.30; y: root.size * 0.86 }
            PathLine { x: root.size * 0.70; y: root.size * 0.86 }
            PathLine { x: root.size * 0.82; y: root.size * 0.42 }
        }
    }

    // ----- network: up/down signal arcs -----
    Shape {
        visible: root.name === "network"
        width: root.size
        height: root.size
        ShapePath {
            strokeColor: root.color
            strokeWidth: root.lw
            capStyle: ShapePath.RoundCap
            joinStyle: ShapePath.RoundJoin
            startX: root.size * 0.50; startY: root.size * 0.78
            PathArc { x: root.size * 0.26; y: root.size * 0.54; radiusX: root.size * 0.24; radiusY: root.size * 0.24 }
            PathMove { x: root.size * 0.50; y: root.size * 0.78 }
            PathArc { x: root.size * 0.34; y: root.size * 0.62; radiusX: root.size * 0.16; radiusY: root.size * 0.16 }
            PathMove { x: root.size * 0.50; y: root.size * 0.78 }
            PathArc { x: root.size * 0.42; y: root.size * 0.70; radiusX: root.size * 0.08; radiusY: root.size * 0.08 }
        }
    }

    // ----- battery: outline + positive terminal -----
    Shape {
        visible: root.name === "battery"
        width: root.size
        height: root.size
        ShapePath {
            strokeColor: root.color
            strokeWidth: root.lw
            capStyle: ShapePath.RoundCap
            joinStyle: ShapePath.RoundJoin
            startX: root.size * 0.20; startY: root.size * 0.24
            PathLine { x: root.size * 0.76; y: root.size * 0.24 }
            PathLine { x: root.size * 0.76; y: root.size * 0.76 }
            PathLine { x: root.size * 0.20; y: root.size * 0.76 }
            PathLine { x: root.size * 0.20; y: root.size * 0.24 }
            PathMove { x: root.size * 0.76; y: root.size * 0.40 }
            PathLine { x: root.size * 0.90; y: root.size * 0.40 }
            PathLine { x: root.size * 0.90; y: root.size * 0.60 }
            PathLine { x: root.size * 0.76; y: root.size * 0.60 }
        }
    }

    // ----- retry: circular arrow -----
    Shape {
        visible: root.name === "retry"
        width: root.size
        height: root.size
        ShapePath {
            strokeColor: root.color
            strokeWidth: root.lw
            capStyle: ShapePath.RoundCap
            joinStyle: ShapePath.RoundJoin
            startX: root.size * 0.50; startY: root.size * 0.78
            PathArc { x: root.size * 0.22; y: root.size * 0.50; radiusX: root.size * 0.28; radiusY: root.size * 0.28 }
            PathLine { x: root.size * 0.16; y: root.size * 0.44 }
            PathLine { x: root.size * 0.22; y: root.size * 0.50 }
            PathLine { x: root.size * 0.16; y: root.size * 0.56 }
        }
    }

    // ----- refresh: circular arrow (other direction) -----
    Shape {
        visible: root.name === "refresh"
        width: root.size
        height: root.size
        ShapePath {
            strokeColor: root.color
            strokeWidth: root.lw
            capStyle: ShapePath.RoundCap
            joinStyle: ShapePath.RoundJoin
            startX: root.size * 0.50; startY: root.size * 0.22
            PathArc { x: root.size * 0.78; y: root.size * 0.50; radiusX: root.size * 0.28; radiusY: root.size * 0.28 }
            PathLine { x: root.size * 0.84; y: root.size * 0.56 }
            PathLine { x: root.size * 0.78; y: root.size * 0.50 }
            PathLine { x: root.size * 0.84; y: root.size * 0.44 }
        }
    }
}