import QtQuick
import QtTest
import "../components"

Item {
    Component {
        id: controllerFactory
        QtObject {
            signal actionSucceeded(string method)
            property bool active: true
            property string state: "ready"
            property bool busy: false
            property int unfinished: 1
            property string errorText: ""
            property var calls: []
            property var snapshot: ({
                    revision: 2,
                    timers: [
                        {
                            id: "one",
                            name: "Tea",
                            state: "running",
                            revision: 2,
                            remainingMs: 300000
                        }
                    ],
                    running: 1,
                    paused: 0,
                    completed: 0
                })
            function start(name, seconds) {
                calls = calls.concat([
                    {
                        name: name,
                        seconds: seconds
                    }
                ]);
            }
            function request(method) {
                calls = calls.concat([method]);
            }
            function act(action, id, rev) {
                calls = calls.concat([
                    {
                        action: action,
                        id: id,
                        revision: rev
                    }
                ]);
            }
            function formatRemaining(ms) {
                return String(ms);
            }
        }
    }
    Component {
        id: factory
        TimerPage {
            width: 368
        }
    }
    TestCase {
        name: "TimerPage"
        property var fake
        property var page
        function init() {
            fake = createTemporaryObject(controllerFactory, parent);
            page = createTemporaryObject(factory, parent, {
                controller: fake
            });
        }
        function test_presets_custom_bounds_and_pending_guards() {
            findChild(page, "preset5").clicked();
            compare(fake.calls[0].seconds, 300);
            findChild(page, "timerMinutes").value = 0;
            findChild(page, "timerSeconds").value = 5;
            findChild(page, "startTimer").clicked();
            compare(fake.calls[1].seconds, 5);
            findChild(page, "timerMinutes").value = 1440;
            findChild(page, "timerSeconds").value = 1;
            verify(!findChild(page, "startTimer").enabled);
            fake.busy = true;
            verify(!findChild(page, "preset5").enabled);
            fake.busy = false;
            fake.unfinished = 10;
            verify(!findChild(page, "preset5").enabled);
            fake.unfinished = 0;
            fake.state = "denied";
            verify(!findChild(page, "preset5").enabled);
            verify(!findChild(page, "startTimer").enabled);
        }
        function test_state_actions_and_stable_rows() {
            const row = findChild(page, "timer-one");
            verify(row !== null);
            findChild(row, "pauseTimer").clicked();
            compare(fake.calls[0].action, "pause");
            compare(fake.calls[0].id, "one");
            compare(fake.calls[0].revision, 2);
            fake.snapshot = {
                revision: 3,
                timers: [
                    {
                        id: "one",
                        name: "Tea",
                        state: "paused",
                        revision: 3,
                        remainingMs: 200000
                    }
                ],
                running: 0,
                paused: 1,
                completed: 0
            };
            compare(findChild(page, "timer-one"), row);
            verify(findChild(row, "resumeTimer").visible);
            findChild(row, "resumeTimer").clicked();
            compare(fake.calls[1].revision, 3);
            fake.snapshot = {
                revision: 4,
                timers: [
                    {
                        id: "one",
                        name: "Tea",
                        state: "completed",
                        revision: 4,
                        remainingMs: 0,
                        notification: "unavailable"
                    }
                ],
                running: 0,
                paused: 0,
                completed: 1
            };
            verify(findChild(row, "dismissTimer").visible);
            verify(!findChild(row, "cancelTimer").visible);
            findChild(row, "dismissTimer").clicked();
            compare(fake.calls[2].action, "dismiss");
        }
    }
}
