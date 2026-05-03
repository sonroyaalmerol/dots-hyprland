pragma Singleton

import qs.modules.common
import qs.modules.common.functions
import qs.services
import QtQuick
import Quickshell

Singleton {
    id: root

    property string killDialogQmlPath: FileUtils.trimFileProtocol(Quickshell.shellPath("killDialog.qml"))

    function load() {
        // dummy to force init
    }

    Connections {
        target: Config
        function onReadyChanged() {
            if (Config.ready) DaemonSocket.conflictCheck()
        }
    }

    Connections {
        target: DaemonSocket
        function onConflictResult(trays, notifications) {
            var openDialog = false;
            if (trays.length > 0) {
                if (!Config.options.conflictKiller.autoKillTrays) openDialog = true;
                else Quickshell.execDetached(["killall", ...trays])
            }
            if (notifications.length > 0) {
                if (!Config.options.conflictKiller.autoKillNotificationDaemons) openDialog = true;
                else Quickshell.execDetached(["killall", ...notifications])
            }
            if (openDialog) {
                Quickshell.execDetached(["qs", "-p", root.killDialogQmlPath])
            }
        }
    }
}