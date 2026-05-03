pragma Singleton

import qs.services
import QtQuick
import Quickshell
import Quickshell.Hyprland
import qs.modules.common

/**
 * Exposes the active Hyprland Xkb keyboard layout name and code for indicators.
 */
Singleton {
	id: root
	property list<string> layoutCodes: DaemonSocket.hyprLayoutCodes
	property var cachedLayoutCodes: ({})
	property string currentLayoutName: DaemonSocket.hyprCurrentLayoutName
	property string currentLayoutCode: DaemonSocket.hyprCurrentLayoutCode

	Connections {
		target: Hyprland
		function onRawEvent(event) {
			if (event.name === "activelayout") {
				const dataString = event.data;
				const newLayout = dataString.substring(dataString.indexOf(",") + 1);
				Config.options.osk.layout = newLayout.split(" (")[0];
			}
		}
	}
}
