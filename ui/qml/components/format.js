.pragma library

// Pure helpers for formatting system monitor values. They hold no state
// and touch no QML types, so they can be unit tested with Qt Test.

// formatBytes returns a human-readable size with one decimal and a unit
// (B, KB, MB, GB, TB). Zero bytes renders as "0 B".
function formatBytes(bytes) {
    if (typeof bytes !== "number" || !isFinite(bytes) || bytes < 0)
        return "0 B";
    if (bytes === 0)
        return "0 B";

    var units = ["B", "KB", "MB", "GB", "TB"];
    var value = bytes;
    var unitIndex = 0;

    while (value >= 1024 && unitIndex < units.length - 1) {
        value /= 1024;
        unitIndex++;
    }

    return value.toFixed(1) + " " + units[unitIndex];
}

// formatPercent returns a percentage with one decimal, clamped to 0-100.
// Non-numeric input renders as "\u2014" (an em dash, meaning "no data").
function formatPercent(value) {
    if (typeof value !== "number" || !isFinite(value))
        return "\u2014";

    if (value < 0)
        value = 0;
    if (value > 100)
        value = 100;

    return value.toFixed(1) + "%";
}

// formatSampledAt returns a short, human-readable age for a timestamp.
// It returns "just now" for sub-minute ages, otherwise "N m ago" or
// "--:--:--" when the timestamp cannot be parsed.
function formatSampledAt(isoString) {
    if (typeof isoString !== "string" || isoString.length === 0)
        return "\u2014";

    var parsed = new Date(isoString);
    if (isNaN(parsed.getTime()))
        return "\u2014";

    var now = new Date();
    var ageMs = now - parsed;
    if (ageMs < 0)
        ageMs = 0;

    var ageSec = Math.floor(ageMs / 1000);
    if (ageSec < 60)
        return "just now";

    var ageMin = Math.floor(ageSec / 60);
    return ageMin + " m ago";
}
