import qs.modules.common
import qs.modules.common.widgets
import qs.services
import QtQuick
import Quickshell

QuickToggleButton {
    id: root
    toggled: DaemonSocket.warpConnected
    visible: DaemonSocket.warpInstalled

    contentItem: CustomIcon {
        source: 'cloudflare-dns-symbolic'
        anchors.centerIn: parent
        width: 16
        height: 16
        colorize: true
        color: root.toggled ? Appearance.m3colors.m3onPrimary : Appearance.colors.colOnLayer1

        Behavior on color {
            animation: Appearance.animation.elementMoveFast.colorAnimation.createObject(this)
        }
    }

    onClicked: {
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

    StyledToolTip {
        text: Translation.tr("Cloudflare WARP (1.1.1.1)")
    }
}