pragma Singleton
pragma ComponentBehavior: Bound

import qs.services
import QtQuick
import Quickshell

Singleton {
	id: root

	property bool tabletMode: false
	property bool textInputActive: false

	// Tri-state mode: "auto" | "tablet" | "desktop"
	property string mode: "auto"
	property bool effectiveTabletMode: {
		if (mode === "tablet") return true
		if (mode === "desktop") return false
		return tabletMode
	}

	function cycleMode() {
		if (root.mode === "auto") root.mode = "tablet"
		else if (root.mode === "tablet") root.mode = "desktop"
		else root.mode = "auto"
	}

	Connections {
		target: DaemonSocket
		function onTabletModeChanged(tablet) { root.tabletMode = tablet }
		function onTextFocusChanged(active) { root.textInputActive = active }
	}
}
