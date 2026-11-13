import QtQuick
import QtTest
import "../components"

Item {
    Component {
        id: controllerFactory
        QtObject {
            property bool active: true
            property bool connected: true
            property string state: "ready"
            property bool dirty: false
            property bool busy: false
            property string pendingMethod: ""
            property var snapshot: ({
                    notes: [
                        {
                            id: "one",
                            title: "Sample",
                            pinned: false
                        }
                    ],
                    canCopy: true
                })
            property string errorText: ""
            property string query: ""
            property string noteID: "one"
            property string title: "Sample"
            property string body: "plain <b>text</b>"
            property bool pinned: false
            property string saveStatus: "Saved"
            property var calls: []
            signal noteOpened
            function create() {
                calls = calls.concat(["create"]);
            }
            function remove() {
                calls = calls.concat(["delete"]);
            }
            function copy() {
                calls = calls.concat(["copy"]);
            }
            function togglePin() {
                pinned = !pinned;
            }
            function open(id) {
                calls = calls.concat([id]);
            }
            function editTitle(value) {
                title = value;
            }
            function editBody(value) {
                body = value;
            }
            function retry() {
            }
            function saveAsNew() {
            }
            function reloadSaved() {
            }
        }
    }
    Component {
        id: factory
        NotesPage {
            width: 368
        }
    }
    TestCase {
        name: "NotesPage"
        property var fake
        property var page
        function init() {
            fake = createTemporaryObject(controllerFactory, parent);
            page = createTemporaryObject(factory, parent, {
                controller: fake
            });
        }
        function test_create_copy_pin_and_confirmed_delete() {
            findChild(page, "newNote").clicked();
            compare(fake.calls[0], "create");
            findChild(page, "copyNote").clicked();
            compare(fake.calls[1], "copy");
            findChild(page, "pinNote").clicked();
            verify(fake.pinned);
            findChild(page, "deleteNote").clicked();
            compare(fake.calls.length, 2);
            const dialog = findChild(page, "deleteNoteDialog");
            verify(dialog.visible);
            dialog.accept();
            compare(fake.calls[2], "delete");
            compare(findChild(page, "noteBody").textFormat, TextEdit.PlainText);
        }
        function test_edit_from_context_action_without_editor_focus() {
            findChild(page, "noteBody").text = "changed by paste action";
            compare(fake.body, "changed by paste action");
        }
        function test_unsaved_permission_and_storage_guards() {
            fake.dirty = true;
            verify(!findChild(page, "newNote").enabled);
            verify(!findChild(page, "copyNote").enabled);
            verify(!findChild(page, "deleteNote").enabled);
            fake.dirty = false;
            fake.snapshot = {
                notes: [],
                canCopy: false,
                storageError: "original preserved"
            };
            verify(!findChild(page, "newNote").enabled);
            verify(!findChild(page, "copyNote").enabled);
        }
    }
}
