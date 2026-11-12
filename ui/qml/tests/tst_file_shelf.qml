import QtQuick
import QtQuick.Window
import QtTest
import "../components"

Item {
    id: host
    width: 420
    height: 800
    property var page
    Component {
        id: factory
        FileShelfPage {
            width: 368
            state: "ready"
        }
    }
    SignalSpy {
        id: addSpy
        target: page
        signalName: "pathsRequested"
    }
    SignalSpy {
        id: copySpy
        target: page
        signalName: "copyRequested"
    }
    SignalSpy {
        id: openSpy
        target: page
        signalName: "folderRequested"
    }
    SignalSpy {
        id: removeSpy
        target: page
        signalName: "removeRequested"
    }
    TestCase {
        name: "FileShelfPage"
        when: windowShown
        function initTestCase() {
            host.Window.window.width = 420;
            host.Window.window.height = 800;
        }
        function init() {
            page = createTemporaryObject(factory, parent, {
                snapshot: {
                    entries: [
                        {
                            id: "1",
                            path: "/tmp/alpha.txt",
                            name: "alpha.txt",
                            directory: false,
                            available: true,
                            size: 20
                        },
                        {
                            id: "2",
                            path: "/tmp/folder",
                            name: "folder",
                            directory: true,
                            available: false,
                            size: 0
                        }
                    ],
                    can_copy: true
                }
            });
            verify(page !== null);
            addSpy.clear();
            copySpy.clear();
            openSpy.clear();
            removeSpy.clear();
            waitForPolish(page);
            waitForRendering(page);
        }
        function test_add_path_and_url_batch() {
            const input = findChild(page, "shelfPath");
            input.text = "/tmp/new file";
            input.forceActiveFocus();
            keyClick(Qt.Key_Return);
            compare(addSpy.signalArguments[0][0][0], "/tmp/new file");
            verify(page.addPaths(["file:///tmp/a%20b", "file:///tmp/folder"]));
            compare(addSpy.signalArguments[1][0].length, 2);
            page.busy = true;
            verify(!page.addPaths(["/tmp/blocked"]));
            compare(addSpy.count, 2);
        }
        function test_drop_accepts_copy_only() {
            const event = {
                accepted: true,
                hasUrls: true,
                urls: ["file:///tmp/a%20b"],
                supportedActions: Qt.CopyAction | Qt.MoveAction,
                action: Qt.IgnoreAction,
                accept: function (action) {
                    this.action = action;
                    this.accepted = true;
                }
            };
            page.handleDrop(event);
            compare(event.action, Qt.CopyAction);
            verify(event.accepted);
            compare(addSpy.signalArguments[0][0][0], "file:///tmp/a%20b");
            event.accepted = true;
            event.supportedActions = Qt.MoveAction;
            page.handleDrop(event);
            verify(!event.accepted);
            compare(addSpy.count, 1);
            event.supportedActions = Qt.CopyAction;
            page.busy = true;
            page.handleDrop(event);
            verify(!event.accepted);
            compare(addSpy.count, 1);
        }
        function test_search_and_row_actions() {
            findChild(page, "shelfSearch").text = "alpha";
            const row = findChild(page, "shelf-1");
            verify(row !== null);
            compare(findChild(page, "shelf-2"), null);
            findChild(row, "shelfCopy").forceActiveFocus();
            keyClick(Qt.Key_Space);
            compare(copySpy.signalArguments[0][0], "1");
            findChild(row, "shelfOpen").forceActiveFocus();
            keyClick(Qt.Key_Return);
            compare(openSpy.signalArguments[0][0], "1");
            findChild(row, "shelfRemove").forceActiveFocus();
            keyClick(Qt.Key_Space);
            compare(removeSpy.signalArguments[0][0], "1");
        }
        function test_missing_item_only_removes() {
            const row = findChild(page, "shelf-2");
            verify(!findChild(row, "shelfOpen").enabled);
            verify(!findChild(row, "shelfCopy").enabled);
            verify(findChild(row, "shelfRemove").enabled);
        }
        function test_without_write_grant() {
            page.snapshot = {
                entries: [
                    {
                        id: "1",
                        path: "/tmp/a",
                        name: "a",
                        directory: false,
                        available: true,
                        size: 1
                    }
                ],
                can_copy: false
            };
            const row = findChild(page, "shelf-1");
            verify(!findChild(row, "shelfCopy").enabled);
            verify(findChild(row, "shelfOpen").enabled);
        }
        function test_refresh_preserves_focus() {
            const row = findChild(page, "shelf-1");
            const button = findChild(row, "shelfRemove");
            button.forceActiveFocus();
            page.snapshot = {
                entries: [
                    {
                        id: "2",
                        path: "/tmp/folder",
                        name: "folder",
                        directory: true,
                        available: false,
                        size: 0
                    },
                    {
                        id: "1",
                        path: "/tmp/alpha.txt",
                        name: "alpha.txt",
                        directory: false,
                        available: true,
                        size: 20
                    }
                ],
                can_copy: true
            };
            compare(findChild(page, "shelf-1"), row);
            verify(button.activeFocus);
        }
        function test_denied_disables_add() {
            page.state = "denied";
            verify(!page.addPaths(["/tmp/a"]));
            verify(!findChild(page, "shelfPath").enabled);
        }
    }
}
