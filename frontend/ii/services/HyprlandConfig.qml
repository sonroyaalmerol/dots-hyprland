pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Hyprland

/**
 * Provides a signal when Hyprland reloads its config.
 * All config set/reset operations should go through DaemonSocket
 * (configSet / configReset / configAnimation) which routes through
 * the daemon's IPC to Hyprland.
 */
Singleton {
	id: root

	signal reloaded()

	Connections {
		target: Hyprland

		function onRawEvent(event) {
			if (event.name == "configreloaded") {
				root.reloaded()
			}
		}
	}
}