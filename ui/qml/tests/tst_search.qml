import QtQuick
import QtQuick.Window
import QtTest
import "../components"
import "../components/search.js" as Search

Item {
    id: host
    width: 420
    height: 800
    property var page
    property var windows: [
        {
            address: "0x1",
            title: "Alpha <b>plain</b>",
            class: "Editor",
            workspace: "2",
            active: true
        },
        {
            address: "0x2",
            title: "Beta note",
            class: "Browser",
            workspace: "3",
            active: false
        }
    ]
    property var apps: [
        {
            id: "alpha.desktop",
            name: "Alpha",
            description: "Editor app",
            keywords: "text;notes;"
        },
        {
            id: "beta.desktop",
            name: "Beta",
            description: "Web browser",
            keywords: "internet;"
        }
    ]
    Component {
        id: factory
        SearchPage {
            width: 368
            windowState: "ready"
            appState: "ready"
        }
    }
    SignalSpy {
        id: focused
        target: page
        signalName: "focusRequested"
    }
    SignalSpy {
        id: launched
        target: page
        signalName: "launchRequested"
    }
    SignalSpy {
        id: highlighted
        target: page
        signalName: "highlightRequested"
    }
    TestCase {
        name: "UnifiedSearch"
        when: windowShown
        function initTestCase() {
            host.Window.window.width = 420;
            host.Window.window.height = 800;
        }
        function init() {
            page = createTemporaryObject(factory, host, {
                windowSnapshot: {
                    windows: host.windows
                },
                appSnapshot: {
                    apps: host.apps
                }
            });
            verify(page !== null);
            focused.clear();
            launched.clear();
            highlighted.clear();
            waitForPolish(page);
            waitForRendering(page);
        }
        function test_matching_ranking_and_namespaced_identity() {
            const rows = Search.results(host.windows, host.apps, "ALPHA");
            compare(rows.length, 2);
            compare(rows[0].kind, "window");
            compare(rows[1].kind, "app");
            const combined = Search.results(host.windows, host.apps, "Editor 2");
            compare(combined.length, 1);
            compare(combined[0].target, "0x1");
            compare(Search.results(host.windows, host.apps, "internet")[0].target, "beta.desktop");
            const ranked = Search.results([], [
                {
                    id: "a.desktop",
                    name: "Other",
                    keywords: "note",
                    description: ""
                },
                {
                    id: "b.desktop",
                    name: "Notebook",
                    description: "",
                    keywords: ""
                },
                {
                    id: "c.desktop",
                    name: "Note",
                    description: "",
                    keywords: ""
                }
            ], "note");
            compare(ranked[0].target, "c.desktop");
            compare(ranked[1].target, "b.desktop");
            compare(ranked[2].target, "a.desktop");
        }
        function test_keyboard_dispatches_selected_kind() {
            const field = findChild(page, "unifiedSearch");
            field.text = "alpha";
            field.forceActiveFocus();
            compare(page.active, true);
            compare(field.activeFocus, true);
            compare(page.selectedKey, "window:0x1");
            keyClick(Qt.Key_Return);
            compare(focused.count, 1);
            compare(focused.signalArguments[0][0], "0x1");
            compare(launched.count, 0);
            keyClick(Qt.Key_Down);
            compare(page.selectedKey, "app:alpha.desktop");
            keyClick(Qt.Key_Return);
            compare(launched.signalArguments[0][0], "alpha.desktop");
            keyClick(Qt.Key_Up);
            compare(page.selectedKey, "window:0x1");
            tryVerify(() => highlighted.count > 0);
            const button = findChild(findChild(page, "search-app:alpha.desktop"), "searchAction");
            button.forceActiveFocus();
            page.focusQuery();
            verify(field.activeFocus);
            compare(field.selectedText, "alpha");
        }
        function test_button_and_busy_guard() {
            const row = findChild(page, "search-app:beta.desktop");
            const button = findChild(row, "searchAction");
            button.forceActiveFocus();
            keyClick(Qt.Key_Space);
            compare(launched.signalArguments[0][0], "beta.desktop");
            page.busy = true;
            verify(!button.enabled);
            page.activate("window:0x1");
            compare(focused.count, 0);
        }
        function test_sources_have_independent_permissions() {
            page.windowState = "denied";
            compare(page.resultCount, 2);
            compare(findChild(page, "search-window:0x1"), null);
            page.activate("window:0x1");
            compare(focused.count, 0);
            page.activate("app:alpha.desktop");
            compare(launched.count, 1);
            page.windowState = "ready";
            page.appState = "disabled";
            compare(page.resultCount, 2);
            page.activate("app:beta.desktop");
            compare(launched.count, 1);
            page.appState = "offline";
            page.windowState = "offline";
            verify(!findChild(page, "unifiedSearch").enabled);
            compare(page.resultCount, 0);
        }
        function test_late_sources_default_to_first_but_preserve_explicit_choice() {
            page.windowState = "pending";
            compare(page.selectedKey, "app:alpha.desktop");
            page.windowState = "ready";
            compare(page.selectedKey, "window:0x1");
            page.windowState = "pending";
            page.moveSelection(1);
            compare(page.selectedKey, "app:beta.desktop");
            page.windowState = "ready";
            compare(page.selectedKey, "app:beta.desktop");
        }
        function test_search_focus_waits_until_a_source_is_ready() {
            page.active = false;
            page.windowState = "pending";
            page.appState = "pending";
            page.active = true;
            const field = findChild(page, "unifiedSearch");
            verify(!field.enabled);
            page.appState = "ready";
            tryVerify(() => field.activeFocus);
        }
        function test_refresh_preserves_row_focus_and_selected_identity() {
            const row = findChild(page, "search-window:0x2");
            const button = findChild(row, "searchAction");
            button.forceActiveFocus();
            compare(page.selectedKey, "window:0x2");
            page.windowSnapshot = {
                windows: [
                    {
                        address: "0x2",
                        title: "Updated",
                        class: "Browser",
                        workspace: "4",
                        active: true
                    }
                ]
            };
            compare(findChild(page, "search-window:0x2"), row);
            verify(button.activeFocus);
            compare(page.selectedKey, "window:0x2");
            page.windowSnapshot = {
                windows: []
            };
            compare(page.selectedKey, "app:alpha.desktop");
            page.activate("window:0x2");
            compare(focused.count, 0);
        }
        function test_bounds_refinement_empty_and_hide_clear_query() {
            const many = [];
            for (let i = 0; i < 50; i++)
                many.push({
                    id: i + ".desktop",
                    name: "App " + i,
                    description: "",
                    keywords: ""
                });
            page.windowSnapshot = {
                windows: []
            };
            page.appSnapshot = {
                apps: many
            };
            compare(page.resultCount, 30);
            compare(page.totalResults, 50);
            const field = findChild(page, "unifiedSearch");
            field.text = "App 49";
            compare(page.resultCount, 1);
            field.text = "no such app";
            compare(page.resultCount, 0);
            field.forceActiveFocus();
            keyClick(Qt.Key_Return);
            compare(launched.count, 0);
            page.active = false;
            compare(field.text, "");
            compare(page.resultCount, 0);
            compare(page.selectedKey, "");
            page.appSnapshot = {
                apps: host.apps
            };
            compare(page.resultCount, 0);
            page.active = true;
            compare(page.resultCount, 2);
        }
    }
}
