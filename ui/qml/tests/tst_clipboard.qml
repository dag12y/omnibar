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
        id: pageFactory
        ClipboardPage {
            width: 360
            state: "ready"
        }
    }
    SignalSpy {
        id: copySpy
        target: page
        signalName: "copyRequested"
    }
    SignalSpy {
        id: deleteSpy
        target: page
        signalName: "deleteRequested"
    }
    SignalSpy {
        id: favoriteSpy
        target: page
        signalName: "favoriteRequested"
    }
    SignalSpy {
        id: collectSpy
        target: page
        signalName: "collectionRequested"
    }
    TestCase {
        name: "ClipboardPage"
        when: windowShown
        function initTestCase() {
            host.Window.window.width = 420;
            host.Window.window.height = 800;
        }
        function init() {
            page = createTemporaryObject(pageFactory, parent, {
                snapshot: {
                    entries: [
                        {
                            id: "1",
                            text: "Alpha <b>plain</b>",
                            copied_at: "2026-11-12T10:00:00Z"
                        },
                        {
                            id: "2",
                            text: "Beta",
                            copied_at: "2026-11-12T11:00:00Z"
                        }
                    ],
                    collecting: false,
                    state: "paused",
                    can_copy: true
                }
            });
            verify(page !== null);
            copySpy.clear();
            deleteSpy.clear();
            collectSpy.clear();
            waitForPolish(page);
            waitForRendering(page);
        }
        function test_search_and_actions() {
            const search = findChild(page, "clipboardSearch");
            search.text = "beta";
            waitForPolish(page);
            waitForRendering(page);
            const row = findChild(page, "history-2");
            verify(row !== null);
            const copy = findChild(row, "copyHistory");
            copy.forceActiveFocus();
            keyClick(Qt.Key_Space);
            compare(copySpy.signalArguments[0][0], "2");
            const remove = findChild(row, "deleteHistory");
            remove.forceActiveFocus();
            keyClick(Qt.Key_Return);
            compare(deleteSpy.signalArguments[0][0], "2");
        }
        function test_collect_is_explicit() {
            compare(collectSpy.count, 0);
            const start = findChild(page, "collectionToggle");
            start.forceActiveFocus();
            keyClick(Qt.Key_Space);
            compare(collectSpy.signalArguments[0][0], true);
            page.snapshot = {
                entries: [],
                collecting: true,
                state: "watching",
                can_copy: true
            };
            keyClick(Qt.Key_Space);
            compare(collectSpy.signalArguments[1][0], false);
        }
        function test_read_only_cannot_copy() {
            page.snapshot = {
                entries: [
                    {
                        id: "1",
                        text: "Read only",
                        copied_at: ""
                    }
                ],
                collecting: false,
                state: "paused",
                can_copy: false
            };
            const row = findChild(page, "history-1");
            verify(!findChild(row, "copyHistory").enabled);
            verify(findChild(row, "deleteHistory").enabled);
        }
        function test_favorites_filter_and_save_action() {
            page.snapshot = {
                entries: [
                    {
                        id: "1",
                        text: "Saved alpha",
                        copied_at: "",
                        favorite: true
                    },
                    {
                        id: "2",
                        text: "Recent beta",
                        copied_at: "",
                        favorite: false
                    }
                ],
                collecting: false,
                state: "paused",
                can_copy: false,
                can_favorite: true
            };
            favoriteSpy.clear();
            const filter = findChild(page, "favoritesFilter");
            filter.forceActiveFocus();
            keyClick(Qt.Key_Space);
            verify(page.favoritesOnly);
            compare(findChild(page, "history-2"), null);
            const row = findChild(page, "history-1");
            verify(row !== null);
            const unsave = findChild(row, "favoriteHistory");
            unsave.forceActiveFocus();
            keyClick(Qt.Key_Space);
            compare(favoriteSpy.signalArguments[0][0], "1");
            compare(favoriteSpy.signalArguments[0][1], false);
            page.snapshot = {
                entries: [
                    {
                        id: "1",
                        text: "Saved alpha",
                        copied_at: "",
                        favorite: false
                    }
                ],
                collecting: false,
                state: "paused",
                can_favorite: true
            };
            compare(findChild(page, "history-1"), null);
        }
        function test_clear_recent_disabled_for_only_favorites() {
            page.snapshot = {
                entries: [
                    {
                        id: "1",
                        text: "Saved",
                        copied_at: "",
                        favorite: true
                    }
                ],
                collecting: false,
                state: "paused",
                can_favorite: true
            };
            verify(!findChild(page, "clearHistory").enabled);
        }
        function test_storage_unavailable_disables_save() {
            page.snapshot = {
                entries: [
                    {
                        id: "1",
                        text: "Recent",
                        copied_at: "",
                        favorite: false
                    }
                ],
                collecting: false,
                state: "paused",
                can_favorite: false,
                favorites_error: "Could not load favorites"
            };
            verify(!findChild(findChild(page, "history-1"), "favoriteHistory").enabled);
            verify(findChild(findChild(page, "history-1"), "deleteHistory").enabled);
        }
        function test_refresh_keeps_focused_entry() {
            const row = findChild(page, "history-1");
            const copy = findChild(row, "copyHistory");
            copy.forceActiveFocus();
            page.snapshot = {
                entries: [
                    {
                        id: "3",
                        text: "Newest",
                        copied_at: ""
                    },
                    {
                        id: "1",
                        text: "Alpha <b>plain</b>",
                        copied_at: "2026-11-12T10:00:00Z"
                    }
                ],
                collecting: true,
                state: "watching",
                can_copy: true
            };
            compare(findChild(page, "history-1"), row);
            verify(copy.activeFocus);
        }
    }
}
