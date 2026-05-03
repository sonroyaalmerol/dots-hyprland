import qs.modules.common
import qs.modules.common.widgets
import qs.services
import Quickshell

QuickToggleButton {
    id: root
    buttonIcon: "gamepad"
    toggled: DaemonSocket.gameModeEnabled

    onClicked: {
        DaemonSocket.gameModeToggle()
    }

    StyledToolTip {
        text: Translation.tr("Game mode")
    }
}