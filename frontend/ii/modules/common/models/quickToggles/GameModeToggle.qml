import QtQuick
import Quickshell.Io
import qs.modules.common.models.hyprland
import qs.services

QuickToggleModel {
	id: root
	name: Translation.tr("Game mode")
	toggled: !confOpt.value
	icon: "gamepad"

	mainAction: () => {
		root.toggled = !root.toggled;
		if (root.toggled) {
			DaemonSocket.configSet("animations:enabled", "0")
			DaemonSocket.configSet("decoration:shadow:enabled", "0")
			DaemonSocket.configSet("decoration:blur:enabled", "0")
			DaemonSocket.configSet("general:gaps_in", "0")
			DaemonSocket.configSet("general:gaps_out", "0")
			DaemonSocket.configSet("general:border_size", "1")
			DaemonSocket.configSet("decoration:rounding", "0")
			DaemonSocket.configSet("general:allow_tearing", "1")
		} else {
			DaemonSocket.configReset("animations:enabled")
			DaemonSocket.configReset("decoration:shadow:enabled")
			DaemonSocket.configReset("decoration:blur:enabled")
			DaemonSocket.configReset("general:gaps_in")
			DaemonSocket.configReset("general:gaps_out")
			DaemonSocket.configReset("general:border_size")
			DaemonSocket.configReset("decoration:rounding")
			DaemonSocket.configReset("general:allow_tearing")
		}
	}

	HyprlandConfigOption {
		id: confOpt
		key: "animations:enabled"
	}

	tooltipText: Translation.tr("Game mode")
}