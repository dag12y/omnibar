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
        WindowPage {
            width: 368
            state: "ready"
        }
    }
    SignalSpy {
        id: focusSpy
        target: page
        signalName: "focusRequested"
    }
    Component {
        id: previewFactory
        QtObject {
            property bool active: true
            property bool busy: false
            property string address: ""
            property string imageSource: ""
            property string errorText: ""
            property int requests: 0
            function request(target) {
                address = target;
                requests++;
            }
            function clear() {
                address = "";
                imageSource = "";
            }
        }
    }
    TestCase {
        name: "WindowPage"
        when: windowShown
        function test_preview_requires_grant_and_never_switches() {
            const preview = createTemporaryObject(previewFactory, parent);
            page.preview = preview;
            const row = findChild(page, "window-0x2");
            const button = findChild(row, "previewWindow");
            verify(!button.enabled);
            page.snapshot = {
                windows: page.snapshot.windows,
                previewsAllowed: true
            };
            button.forceActiveFocus();
            keyClick(Qt.Key_Space);
            compare(preview.address, "0x2");
            compare(preview.requests, 1);
            compare(focusSpy.count, 0);
        }
        function initTestCase() {
            host.Window.window.width = 420;
            host.Window.window.height = 800;
        }
        function init() {
            page = createTemporaryObject(factory, parent, {
                snapshot: {
                    windows: [
                        {
                            address: "0x1",
                            title: "Alpha <b>plain</b>",
                            class: "Browser",
                            workspace: "1",
                            monitor: 0,
                            active: true
                        },
                        {
                            address: "0x2",
                            title: "Beta note",
                            class: "Editor",
                            workspace: "2",
                            monitor: 0,
                            active: false
                        }
                    ]
                }
            });
            verify(page !== null);
            focusSpy.clear();
            waitForPolish(page);
            waitForRendering(page);
        }
        function test_search_by_app_title_workspace_and_enter() {
            const search = findChild(page, "windowSearch");
            search.text = "editor";
            search.forceActiveFocus();
            keyClick(Qt.Key_Return);
            compare(focusSpy.signalArguments[0][0], "0x2");
            compare(findChild(page, "window-0x1"), null);
            search.text = "alpha";
            keyClick(Qt.Key_Return);
            compare(focusSpy.signalArguments[1][0], "0x1");
            search.text = "2";
            keyClick(Qt.Key_Return);
            compare(focusSpy.signalArguments[2][0], "0x2");
        }
        function test_switch_and_busy() {
            const row = findChild(page, "window-0x2");
            const button = findChild(row, "switchWindow");
            button.forceActiveFocus();
            keyClick(Qt.Key_Space);
            compare(focusSpy.signalArguments[0][0], "0x2");
            page.busy = true;
            verify(!button.enabled);
        }
        function test_refresh_preserves_focus_and_removed_rows_disappear() {
            const row = findChild(page, "window-0x2");
            const button = findChild(row, "switchWindow");
            button.forceActiveFocus();
            page.snapshot = {
                windows: [
                    {
                        address: "0x2",
                        title: "Updated",
                        class: "Editor",
                        workspace: "3",
                        monitor: 1,
                        active: true
                    }
                ]
            };
            compare(findChild(page, "window-0x2"), row);
            verify(button.activeFocus);
            compare(findChild(page, "window-0x1"), null);
        }
        function test_denied_and_empty_do_not_focus() {
            page.state = "denied";
            verify(!findChild(page, "windowSearch").enabled);
            page.state = "ready";
            page.snapshot = {
                windows: []
            };
            const search = findChild(page, "windowSearch");
            search.forceActiveFocus();
            keyClick(Qt.Key_Return);
            compare(focusSpy.count, 0);
        }
    }
}
