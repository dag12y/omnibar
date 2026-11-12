// Return a new object so QML bindings observe every live reading. Never mutate
// the cached object: a parent binding may otherwise keep the old field values.
function applySystemEvent(previous, event, data) {
    const snap = Object.assign({}, previous || {})
    if (event === "cpu") snap.cpu = data
    else if (event === "memory") {
        snap.memory = data
        if (data.swap_total !== undefined)
            snap.swap = {total: data.swap_total, used: data.swap_used}
    } else if (event === "disk") snap.disk = data
    else if (event === "network") snap.network = data
    else if (event === "battery") snap.battery = data
    snap.sampled_at = new Date().toISOString()
    return snap
}
