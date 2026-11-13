import QtQuick
import QtTest
import "../components"

Item {
    Component {
        id: factory
        WindowControls {
            width: 368
            address: "0x2"
            workspaceID: 1
        }
    }
    TestCase {
        name: "WindowControls"
        property var controls
        SignalSpy {
            id: requested
            target: parent.controls
            signalName: "controlRequested"
        }
        function init() {
            controls = createTemporaryObject(factory, parent);
            requested.clear();
        }
        function test_explicit_desired_states_and_workspace() {
            findChild(controls, "toggleFloating").clicked();
            compare(requested.signalArguments[0][0], "windows.setFloating");
            compare(requested.signalArguments[0][1].address, "0x2");
            compare(requested.signalArguments[0][1].enabled, true);
            controls.floating = true;
            findChild(controls, "toggleFloating").clicked();
            compare(requested.signalArguments[1][1].enabled, false);
            findChild(controls, "toggleMaximized").clicked();
            compare(requested.signalArguments[2][1].enabled, true);
            controls.fullscreen = 1;
            findChild(controls, "toggleMaximized").clicked();
            compare(requested.signalArguments[3][1].enabled, false);
            controls.fullscreen = 0;
            findChild(controls, "destinationWorkspace").value = 2;
            findChild(controls, "moveWorkspace").clicked();
            compare(requested.signalArguments[4][1].workspace, 2);
        }
        function test_busy_and_fullscreen_disable_actions() {
            controls.busy = true;
            verify(!findChild(controls, "toggleFloating").enabled);
            verify(!findChild(controls, "toggleMaximized").enabled);
            controls.busy = false;
            controls.fullscreen = 2;
            verify(!findChild(controls, "toggleFloating").enabled);
            verify(!findChild(controls, "toggleMaximized").enabled);
            verify(!findChild(controls, "moveWorkspace").enabled);
        }
    }
}
