import QtQuick

Canvas {
    id: root
    property var samples: []
    property color stroke: Theme.accent
    onSamplesChanged: requestPaint()
    onStrokeChanged: requestPaint()
    onWidthChanged: requestPaint()
    onHeightChanged: requestPaint()
    onPaint: {
        const ctx = getContext("2d")
        ctx.reset()
        if (samples.length < 2) return
        ctx.beginPath()
        for (let i = 0; i < samples.length; i++) {
            const x = i * width / (samples.length - 1)
            const y = height - 3 - Math.max(0, Math.min(100, samples[i])) / 100 * (height - 6)
            if (i === 0) ctx.moveTo(x, y)
            else ctx.lineTo(x, y)
        }
        ctx.strokeStyle = stroke
        ctx.lineWidth = 2
        ctx.lineJoin = "round"
        ctx.stroke()
        ctx.lineTo(width, height)
        ctx.lineTo(0, height)
        ctx.closePath()
        const fill = ctx.createLinearGradient(0, 0, 0, height)
        fill.addColorStop(0, Qt.alpha(stroke, 0.25))
        fill.addColorStop(1, Qt.alpha(stroke, 0))
        ctx.fillStyle = fill
        ctx.fill()
    }
}
