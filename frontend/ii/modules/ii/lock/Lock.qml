pragma ComponentBehavior: Bound
import qs
import qs.services
import qs.modules.common
import qs.modules.common.functions
import qs.modules.common.panels.lock
import QtQuick
import Quickshell
import Quickshell.Wayland

LockScreen {
    id: root

    // Monitor name -> workspace id to restore on unlock (set when locking)
    property var savedWorkspaces: ({})

    Timer {
        id: restoreTimer
        interval: 150
        repeat: false
        onTriggered: {
            for (var j = 0; j < Quickshell.screens.length; ++j) {
                var monName = Quickshell.screens[j].name
                var wsId = root.savedWorkspaces[monName]
                if (wsId !== undefined) {
                    DaemonSocket.monitorFocus(monName)
                    DaemonSocket.workspaceFocus(wsId.toString())
                }
            }
            DaemonSocket.reload()
        }
    }

    lockSurface: LockSurface {
        context: root.context
    }

    // Lock and unlock via individual daemon commands
    Connections {
        target: GlobalStates
        function onScreenLockedChanged() {
            if (GlobalStates.screenLocked) {
                // Lock: save workspace per monitor and move all to temp workspace
                var next = {}
                for (var i = 0; i < Quickshell.screens.length; ++i) {
                    var mon = Quickshell.screens[i].name
                    var mData = HyprlandData.monitors.find(m => m.name === mon)
                    if (mData?.activeWorkspace == undefined) {
                        return;
                    }
                    var ws = (mData?.activeWorkspace?.id ?? 1)
                    next[mon] = ws
                }
                root.savedWorkspaces = next
                DaemonSocket.configAnimation("workspaces", "true", "7", "menu_decel", "slidevert")
                for (i = 0; i < Quickshell.screens.length; ++i) {
                    mon = Quickshell.screens[i].name
                    ws = next[mon]
                    DaemonSocket.monitorFocus(mon)
                    DaemonSocket.workspaceFocus((2147483647 - ws).toString())
                }
                DaemonSocket.reload()
            } else {
                restoreTimer.start()
            }
        }
    }

    // Push everything down (visual only; workspace switch is in Connections above)
    Variants {
        model: Quickshell.screens
        delegate: Scope {
            required property ShellScreen modelData
            property bool shouldPush: GlobalStates.screenLocked
            property string targetMonitorName: modelData.name
            property int verticalMovementDistance: modelData.height
            property int horizontalSqueeze: modelData.width * 0.2
        }
    }
}
