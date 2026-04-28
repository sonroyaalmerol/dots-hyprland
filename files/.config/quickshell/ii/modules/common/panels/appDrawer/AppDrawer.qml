pragma ComponentBehavior: Bound
import QtQuick
import Quickshell
import Quickshell.Hyprland
import Quickshell.Io
import qs
import qs.services
import qs.modules.common

Scope {
    id: root

    Connections {
        target: GlobalStates
        function onAppDrawerOpenChanged() {
            if (GlobalStates.appDrawerOpen) panelLoader.active = true;
        }
    }

    Loader {
        id: panelLoader
        active: GlobalStates.appDrawerOpen
        sourceComponent: PanelWindow {
            id: panelWindow
            exclusiveZone: 0
            WlrLayershell.namespace: "quickshell:appDrawer"
            WlrLayershell.keyboardFocus: WlrKeyboardFocus.OnDemand
            WlrLayershell.layer: WlrLayer.Top
            color: "transparent"
            anchors { top: true; bottom: true; left: true; right: true }

            HyprlandFocusGrab {
                active: true
                windows: [panelWindow]
                onCleared: content.close()
            }

            AppDrawerContent {
                id: content
                anchors.fill: parent
                focus: true
                onClosed: {
                    GlobalStates.appDrawerOpen = false;
                    panelLoader.active = false;
                }
            }
        }
    }

    GlobalShortcut {
        name: "appDrawerToggle"
        description: "Toggles the app drawer"
        onPressed: {
            GlobalStates.appDrawerOpen = !GlobalStates.appDrawerOpen;
        }
    }

    IpcHandler {
        target: "appDrawer"
        function toggle() { GlobalStates.appDrawerOpen = !GlobalStates.appDrawerOpen; }
        function open() { GlobalStates.appDrawerOpen = true; }
        function close() { GlobalStates.appDrawerOpen = false; }
    }
}