import QtQuick
import qs.services
import qs.modules.common
import qs.modules.common.functions
import qs.modules.common.widgets
import Quickshell

QuickToggleModel {
    id: root
    name: Translation.tr("Cloudflare WARP")

    toggled: DaemonSocket.warpConnected
    icon: "cloud_lock"
    available: DaemonSocket.warpInstalled

    mainAction: () => {
        DaemonSocket.warpToggle()
    }

    Connections {
        target: DaemonSocket
        function onWarpStatusUpdated() {
            if (DaemonSocket.warpStatus.includes("Unable")) {
                DaemonSocket.warpRegister()
            }
        }
    }

    tooltipText: Translation.tr("Cloudflare WARP (1.1.1.1)")
}