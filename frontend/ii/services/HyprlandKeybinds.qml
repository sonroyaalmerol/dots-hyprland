pragma Singleton
pragma ComponentBehavior: Bound

import qs.modules.common
import qs.modules.common.functions
import qs.services
import QtQuick
import Quickshell
import Quickshell.Hyprland

/**
 * A service that provides access to Hyprland keybinds.
 * Reads parsed keybinds from the daemon.
 */
Singleton {
	id: root
	property var defaultKeybinds: {"children": []}
	property var userKeybinds: {"children": []}
	property var keybinds: ({
		children: [
			...(defaultKeybinds.children ?? []),
			...(userKeybinds.children ?? []),
		]
	})

	Connections {
		target: DaemonSocket

		function onHyprKeybindsUpdated() {
			try { root.defaultKeybinds = JSON.parse(DaemonSocket.hyprDefaultKeybinds) } catch(e) {}
			try { root.userKeybinds = JSON.parse(DaemonSocket.hyprUserKeybinds) } catch(e) {}
		}
	}

	Connections {
		target: Hyprland
		function onRawEvent(event) {
			if (event.name == "configreloaded") {
				DaemonSocket.sendCommand("reload-keybinds")
			}
		}
	}
}
