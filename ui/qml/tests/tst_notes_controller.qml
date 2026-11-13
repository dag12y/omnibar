import QtQuick
import QtTest
import "../components"

Item {
    Component {
        id: clientFactory
        QtObject {
            property bool handshakeComplete: true
            property bool backendConnected: true
            property string status: "connected"
            property int nextRequestId: 1
            signal handshakeSucceeded
            signal messageReceived(var message)
            property QtObject socket: QtObject {
                property bool connected: true
                property var writes: []
                function write(line) {
                    writes = writes.concat([JSON.parse(line)]);
                }
                function flush() {
                }
            }
        }
    }
    Component {
        id: factory
        NotesController {}
    }
    TestCase {
        name: "NotesController"
        property var fake
        property var controller
        function note(body, revision) {
            return {
                id: "one",
                title: "Draft",
                body: body,
                pinned: false,
                revision: revision
            };
        }
        function result(n) {
            return {
                revision: n ? n.revision : 2,
                notes: [
                    {
                        id: "one",
                        title: "Draft",
                        pinned: false,
                        revision: n ? n.revision : 2
                    }
                ],
                note: n,
                canCopy: true
            };
        }
        function reply(value) {
            fake.messageReceived({
                jsonrpc: "2.0",
                id: controller.pendingID,
                result: value
            });
        }
        function error(code) {
            fake.messageReceived({
                jsonrpc: "2.0",
                id: controller.pendingID,
                error: {
                    code: code,
                    message: "save failed"
                }
            });
        }
        function last() {
            return fake.socket.writes[fake.socket.writes.length - 1];
        }
        function init() {
            fake = createTemporaryObject(clientFactory, parent);
            controller = createTemporaryObject(factory, parent, {
                client: fake,
                active: true
            });
            reply(result(null));
            controller.open("one");
            reply(result(note("original", 2)));
        }
        function test_debounce_and_newer_edits_during_save() {
            controller.editBody("first");
            controller.editBody("second");
            compare(controller.saveStatus, "Unsaved");
            wait(800);
            compare(last().method, "notes.update");
            compare(last().params.body, "second");
            compare(controller.saveStatus, "Saving…");
            controller.editBody("third");
            reply(result(note("second", 3)));
            verify(controller.dirty);
            compare(controller.body, "third");
            compare(controller.noteRevision, 3);
            wait(800);
            compare(last().params.revision, 3);
            compare(last().params.body, "third");
            reply(result(note("third", 4)));
            verify(!controller.dirty);
            compare(controller.saveStatus, "Saved");
        }
        function test_hide_flush_failure_and_draft_retention() {
            controller.editBody("keep me");
            controller.active = false;
            compare(last().method, "notes.update");
            error(-32603);
            verify(controller.dirty);
            compare(controller.body, "keep me");
            controller.active = true;
            compare(last().method, "notes.get");
            reply(result(note("original", 2)));
            compare(last().method, "notes.update");
            reply(result(note("keep me", 3)));
            verify(!controller.dirty);
        }
        function test_conflict_never_overwrites_and_can_save_as_new() {
            controller.editBody("local");
            controller.save();
            error(-32004);
            verify(controller.conflicted);
            verify(controller.dirty);
            const count = fake.socket.writes.length;
            controller.save();
            compare(fake.socket.writes.length, count);
            controller.retry();
            reply(result(note("elsewhere", 3)));
            compare(controller.body, "local");
            verify(controller.conflicted);
            controller.saveAsNew();
            compare(last().method, "notes.create");
            compare(last().params.body, "local");
            const copied = note("local", 4);
            copied.id = "two";
            reply(result(copied));
            compare(controller.noteID, "two");
            verify(!controller.dirty);
        }
        function test_lost_save_ack_reconciles_after_reconnect() {
            controller.editBody("ack lost");
            controller.save();
            const lateID = controller.pendingID;
            fake.backendConnected = false;
            fake.status = "offline";
            verify(controller.dirty);
            compare(controller.state, "offline");
            fake.messageReceived({
                jsonrpc: "2.0",
                id: lateID,
                result: result(note("ack lost", 3))
            });
            verify(controller.dirty);
            fake.backendConnected = true;
            fake.status = "connected";
            fake.handshakeSucceeded();
            compare(last().method, "notes.get");
            reply(result(note("ack lost", 3)));
            verify(!controller.dirty);
            compare(controller.noteRevision, 3);
        }
        function test_late_load_cannot_replace_new_edits() {
            controller.refresh();
            reply(result(note("remote", 3)));
            compare(last().method, "notes.get");
            controller.editBody("typed while loading");
            reply(result(note("remote", 3)));
            compare(controller.body, "typed while loading");
            verify(controller.dirty);
            verify(controller.recoveryNeeded);
        }
        function test_search_does_not_clear_selected_editor_and_copy_requires_save() {
            controller.query = "missing";
            wait(250);
            reply({
                revision: 2,
                notes: [],
                canCopy: true
            });
            compare(controller.noteID, "one");
            controller.editBody("dirty");
            const count = fake.socket.writes.length;
            controller.copy();
            controller.open("two");
            compare(fake.socket.writes.length, count);
            compare(controller.body, "dirty");
        }
        function test_pin_delete_revision_and_clean_shutdown_state() {
            controller.togglePin();
            controller.save();
            verify(last().params.pinned);
            const pinned = note("original", 3);
            pinned.pinned = true;
            reply(result(pinned));
            controller.remove();
            compare(last().params.revision, 3);
            reply({
                revision: 4,
                notes: [],
                canCopy: true
            });
            compare(controller.noteID, "");
            verify(!controller.dirty);
        }
    }
}
