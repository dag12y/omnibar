.pragma library

// Keep the two target types distinct: app names are not window identities.
function results(windows, apps, query) {
    const tokens = query.trim().toLowerCase().split(/\s+/).filter(token => token.length > 0);
    const scored = [];
    function add(row, primary, extra) {
        const name = primary.toLowerCase();
        const haystack = (primary + " " + extra).toLowerCase();
        if (!tokens.every(token => haystack.includes(token))) return;
        row.score = tokens.reduce((score, token) => score + (name === token ? 0 : name.startsWith(token) ? 1 : name.includes(token) ? 2 : 3), 0);
        scored.push(row);
    }
    for (const window of windows || []) {
        add({
            key: "window:" + window.address,
            kind: "window",
            target: window.address,
            title: window.title,
            detail: window.class + " · Workspace " + (window.workspace || "?"),
            active: window.active === true
        }, window.title, window.class + " " + window.workspace);
    }
    for (const app of apps || []) {
        add({
            key: "app:" + app.id,
            kind: "app",
            target: app.id,
            title: app.name,
            detail: app.description || "Installed application",
            active: false
        }, app.name, (app.description || "") + " " + (app.keywords || ""));
    }
    scored.sort((a, b) => {
        // Running matches come first, so a launch never silently replaces Switch.
        if (a.kind !== b.kind) return a.kind === "window" ? -1 : 1;
        if (a.score !== b.score) return a.score - b.score;
        const left = a.title.toLowerCase(), right = b.title.toLowerCase();
        if (left !== right) return left < right ? -1 : 1;
        return a.key < b.key ? -1 : a.key === b.key ? 0 : 1;
    });
    return scored;
}
