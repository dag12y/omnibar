import QtQuick
import "protocol.js" as Protocol

Item {
    id: root
    required property var client
    property bool active: false
    property string state: "pending"
    property string errorText: ""
    property var snapshot: ({
            notes: [],
            revision: 0,
            canCopy: false
        })
    property string query: ""
    property string noteID: ""
    property string title: ""
    property string body: ""
    property bool pinned: false
    property double noteRevision: 0
    property int editVersion: 0
    property int savedVersion: 0
    readonly property bool dirty: editVersion !== savedVersion
    property bool conflicted: false
    property bool recoveryNeeded: false
    property bool busy: false
    property int pendingID: -1
    property string pendingMethod: ""
    property var pending: ({})
    readonly property bool connected: client.handshakeComplete && client.backendConnected && client.socket.connected
    readonly property string saveStatus: busy && (pendingMethod === "notes.update" || pendingMethod === "notes.create") ? "Saving…" : dirty ? "Unsaved" : noteID ? "Saved" : ""
    signal noteOpened
    signal copied

    function send(method, params, info) {
        if (busy || !connected)
            return false;
        pendingID = client.nextRequestId++;
        pendingMethod = method;
        pending = Object.assign({
            version: editVersion,
            editor: noteID,
            query: query
        }, info || {});
        busy = true;
        const payload = Object.assign({}, params || {}, {
            query: query
        });
        client.socket.write(Protocol.buildRequest(pendingID, method, payload) + "\n");
        client.socket.flush();
        deadline.restart();
        return true;
    }
    function refresh() {
        if (!connected || busy)
            return;
        if (dirty && recoveryNeeded && noteID)
            send("notes.get", {
                id: noteID
            }, {
                reconcile: true
            });
        else
            send("notes.getSnapshot");
    }
    function changed() {
        editVersion++;
        if (!conflicted)
            errorText = "";
        autosave.restart();
    }
    function editTitle(value) {
        if (title !== value) {
            title = value;
            changed();
        }
    }
    function editBody(value) {
        if (body !== value) {
            body = value;
            changed();
        }
    }
    function togglePin() {
        if (noteID) {
            pinned = !pinned;
            changed();
        }
    }
    function applyNote(note) {
        noteID = note.id;
        title = note.title;
        body = note.body;
        pinned = note.pinned;
        noteRevision = note.revision;
        editVersion++;
        savedVersion = editVersion;
        conflicted = false;
        recoveryNeeded = false;
        errorText = "";
    }
    function clearEditor() {
        noteID = "";
        title = "";
        body = "";
        pinned = false;
        noteRevision = 0;
        editVersion++;
        savedVersion = editVersion;
        conflicted = false;
        recoveryNeeded = false;
    }
    function open(id) {
        if (dirty || busy) {
            errorText = "Save or reload this draft before opening another note.";
            return;
        }
        send("notes.get", {
            id: id
        }, {
            open: true
        });
    }
    function create() {
        if (dirty || busy || snapshot.storageError)
            return;
        send("notes.create", {
            title: "Untitled note",
            body: ""
        }, {
            version: editVersion,
            editor: noteID,
            create: true
        });
    }
    function saveAsNew() {
        if (busy || !dirty || snapshot.storageError)
            return;
        if (!validDraft())
            return;
        send("notes.create", {
            title: title.trim(),
            body: body
        }, {
            version: editVersion,
            editor: noteID,
            create: true
        });
    }
    function validDraft() {
        if (!title.trim()) {
            errorText = "Give this note a title before saving.";
            return false;
        }
        return true;
    }
    function save() {
        if (!dirty || busy || !connected || conflicted || !noteID || snapshot.storageError)
            return;
        if (recoveryNeeded) {
            refresh();
            return;
        }
        if (!validDraft())
            return;
        send("notes.update", {
            id: noteID,
            revision: noteRevision,
            title: title.trim(),
            body: body,
            pinned: pinned
        }, {
            version: editVersion,
            editor: noteID,
            title: title.trim()
        });
    }
    function retry() {
        errorText = "";
        if (dirty && noteID) {
            recoveryNeeded = true;
            refresh();
        } else
            refresh();
    }
    function reloadSaved() {
        if (!busy && noteID)
            send("notes.get", {
                id: noteID
            }, {
                open: true,
                discard: true
            });
    }
    function remove() {
        if (!dirty && !busy && noteID)
            send("notes.delete", {
                id: noteID,
                revision: noteRevision
            }, {
                editor: noteID
            });
    }
    function copy() {
        if (!dirty && !busy && noteID && body && snapshot.canCopy)
            send("notes.copy", {
                id: noteID,
                revision: noteRevision
            });
    }
    onActiveChanged: {
        if (active)
            refresh();
        else if (dirty)
            save();
    }
    onQueryChanged: searchDelay.restart()
    Timer {
        id: searchDelay
        interval: 200
        onTriggered: if (root.active)
            root.refresh()
    }
    Timer {
        id: autosave
        interval: 700
        onTriggered: root.save()
    }
    Timer {
        interval: 3000
        repeat: true
        running: root.active && root.connected && root.state === "ready"
        onTriggered: if (!root.busy)
            root.send("notes.getSnapshot")
    }
    Timer {
        id: deadline
        interval: 8000
        onTriggered: {
            root.recoveryNeeded = root.dirty;
            root.pendingID = -1;
            root.busy = false;
            root.errorText = "Response timed out. Your draft is kept; Retry checks the saved version.";
        }
    }
    Connections {
        target: root.client
        function onHandshakeSucceeded() {
            root.pendingID = -1;
            root.busy = false;
            deadline.stop();
            root.snapshot = {
                notes: [],
                revision: 0,
                canCopy: false
            };
            root.state = "pending";
            root.recoveryNeeded = root.dirty;
            if (root.active || root.dirty)
                root.refresh();
        }
        function onStatusChanged() {
            if (!root.connected) {
                root.pendingID = -1;
                root.busy = false;
                deadline.stop();
                root.state = "offline";
                root.snapshot = {
                    notes: [],
                    revision: 0,
                    canCopy: false
                };
                root.recoveryNeeded = root.dirty;
                root.errorText = root.dirty ? "Backend unavailable. Unsaved draft kept in this UI." : "Backend unavailable.";
            }
        }
        function onMessageReceived(message) {
            const parsed = Protocol.classify(message);
            if (parsed.kind !== "response" || parsed.id !== root.pendingID)
                return;
            const method = root.pendingMethod;
            const info = root.pending;
            deadline.stop();
            root.pendingID = -1;
            root.busy = false;
            if (parsed.error) {
                root.errorText = parsed.error.message;
                if (parsed.error.code === -32001)
                    root.state = "denied";
                else if (parsed.error.code === -32002)
                    root.state = "disabled";
                else if (root.state === "pending")
                    root.state = "unavailable";
                if (parsed.error.code === -32004)
                    root.conflicted = true;
                if (method === "notes.update")
                    root.recoveryNeeded = true;
                autosave.stop();
                return;
            }
            const result = parsed.result;
            root.state = "ready";
            if (result.revision >= root.snapshot.revision && info.query === root.query)
                root.snapshot = result;
            if (method === "notes.get" && info.reconcile) {
                const note = result.note;
                if (note.title === root.title.trim() && note.body === root.body && note.pinned === root.pinned) {
                    root.noteRevision = note.revision;
                    root.savedVersion = root.editVersion;
                    root.conflicted = false;
                    root.recoveryNeeded = false;
                    root.errorText = "";
                } else if (note.revision === root.noteRevision) {
                    root.recoveryNeeded = false;
                    root.conflicted = false;
                    root.save();
                } else {
                    root.conflicted = true;
                    root.recoveryNeeded = false;
                    root.errorText = "This note changed elsewhere. Reload saved or save this draft as a new note.";
                }
            } else if (method === "notes.get" || method === "notes.create") {
                if (method === "notes.get" && root.dirty && info.version !== root.editVersion) {
                    root.recoveryNeeded = true;
                    root.errorText = "Edits kept while the saved note was loading. Retry to check its revision.";
                } else if (method === "notes.get" || info.version === root.editVersion)
                    root.applyNote(result.note);
                else {
                    root.noteID = result.note.id;
                    root.noteRevision = result.note.revision;
                    root.savedVersion = info.version;
                }
                root.noteOpened();
            } else if (method === "notes.update" && root.noteID === info.editor) {
                root.noteRevision = result.note.revision;
                root.savedVersion = info.version;
                if (!root.dirty)
                    root.title = result.note.title;
                root.conflicted = false;
                root.recoveryNeeded = false;
                root.errorText = "";
            } else if (method === "notes.delete" && root.noteID === info.editor) {
                root.clearEditor();
                root.errorText = "";
            } else if (method === "notes.copy")
                root.copied();
            if (method === "notes.getSnapshot" && root.noteID) {
                const summary = result.notes.find(n => n.id === root.noteID);
                // A search filter can hide the selected note; only an unfiltered
                // snapshot can prove deletion. Dirty text is never replaced.
                if (!summary && !root.query && !root.dirty)
                    root.clearEditor();
                else if (summary && summary.revision !== root.noteRevision && !root.dirty)
                    root.send("notes.get", {
                        id: root.noteID
                    }, {
                        open: true
                    });
            }
            if (root.dirty && !root.conflicted && !root.recoveryNeeded)
                autosave.restart();
        }
    }
}
