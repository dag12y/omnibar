import QtQuick
import QtTest
import "../components"

Item {
    Component {
        id: factory
        LayoutPanel {
            width: 368
            active: true
        }
    }
    TestCase {
        name: "SavedLayouts"
        property var panel
        SignalSpy {
            id: requested
            target: parent.panel
            signalName: "layoutRequested"
        }
        function init() {
            panel = createTemporaryObject(factory, parent, {
                snapshot: {
                    windows: [
                        {
                            active: true,
                            workspaceID: 2
                        }
                    ],
                    layouts: [
                        {
                            id: "abc",
                            name: "Desk",
                            count: 2
                        }
                    ]
                }
            });
            requested.clear();
        }
        function test_save_restore_delete_and_selection_stability() {
            findChild(panel, "layoutName").text = "My desk";
            findChild(panel, "saveLayout").clicked();
            compare(requested.signalArguments[0][0], "windows.saveLayout");
            compare(requested.signalArguments[0][1].name, "My desk");
            compare(requested.signalArguments[0][1].workspace, 2);
            panel.snapshot = {
                windows: [],
                layouts: [
                    {
                        id: "def",
                        name: "Another",
                        count: 1
                    },
                    {
                        id: "abc",
                        name: "Desk",
                        count: 2
                    }
                ]
            };
            compare(panel.selectedID, "abc");
            findChild(panel, "restoreLayout").clicked();
            compare(requested.signalArguments[1][0], "windows.restoreLayout");
            compare(requested.signalArguments[1][1].id, "abc");
            findChild(panel, "deleteLayout").clicked();
            compare(requested.signalArguments[2][0], "windows.deleteLayout");
        }
        function test_busy_error_empty_and_hide() {
            panel.busy = true;
            verify(!findChild(panel, "restoreLayout").enabled);
            panel.busy = false;
            panel.snapshot = {
                windows: [],
                layouts: [],
                layoutError: "Original file preserved"
            };
            verify(!findChild(panel, "saveLayout").enabled);
            verify(!findChild(panel, "restoreLayout").enabled);
            findChild(panel, "layoutName").text = "Private name";
            panel.active = false;
            compare(findChild(panel, "layoutName").text, "");
            compare(panel.selectedID, "");
        }
    }
}
