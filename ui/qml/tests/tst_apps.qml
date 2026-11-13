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
        AppPage {
            width: 368
            state: "ready"
        }
    }
    SignalSpy {
        id: launchSpy
        target: page
        signalName: "launchRequested"
    }
    TestCase {
        name: "AppPage"
        when: windowShown
        function initTestCase() {
            host.Window.window.width = 420;
            host.Window.window.height = 800;
        }
        function init() {
            page = createTemporaryObject(factory, parent, {
                snapshot: {
                    apps: [
                        {
                            id: "alpha.desktop",
                            name: "Alpha <b>plain</b>",
                            description: "Browser",
                            keywords: "1"
                        },
                        {
                            id: "beta.desktop",
                            name: "Beta note",
                            description: "Editor",
                            keywords: "2"
                        }
                    ]
                }
            });
            verify(page !== null);
            launchSpy.clear();
            waitForPolish(page);
            waitForRendering(page);
        }
        function test_search_by_name_description_keywords_and_enter() {
            const search = findChild(page, "appSearch");
            search.text = "editor";
            search.forceActiveFocus();
            keyClick(Qt.Key_Return);
            compare(launchSpy.signalArguments[0][0], "beta.desktop");
            compare(findChild(page, "app-alpha.desktop"), null);
            search.text = "alpha";
            keyClick(Qt.Key_Return);
            compare(launchSpy.signalArguments[1][0], "alpha.desktop");
            search.text = "2";
            keyClick(Qt.Key_Return);
            compare(launchSpy.signalArguments[2][0], "beta.desktop");
        }
        function test_launch_and_busy() {
            const row = findChild(page, "app-beta.desktop");
            const button = findChild(row, "launchApp");
            button.forceActiveFocus();
            keyClick(Qt.Key_Space);
            compare(launchSpy.signalArguments[0][0], "beta.desktop");
            page.busy = true;
            verify(!button.enabled);
        }
        function test_refresh_preserves_focus_and_removed_rows_disappear() {
            const row = findChild(page, "app-beta.desktop");
            const button = findChild(row, "launchApp");
            button.forceActiveFocus();
            page.snapshot = {
                apps: [
                    {
                        id: "beta.desktop",
                        name: "Updated",
                        description: "Editor",
                        keywords: "3"
                    }
                ]
            };
            compare(findChild(page, "app-beta.desktop"), row);
            verify(button.activeFocus);
            compare(findChild(page, "app-alpha.desktop"), null);
        }
        function test_denied_and_empty_do_not_focus() {
            page.state = "denied";
            verify(!findChild(page, "appSearch").enabled);
            page.state = "ready";
            page.snapshot = {
                apps: []
            };
            const search = findChild(page, "appSearch");
            search.forceActiveFocus();
            keyClick(Qt.Key_Return);
            compare(launchSpy.count, 0);
        }
    }
}
