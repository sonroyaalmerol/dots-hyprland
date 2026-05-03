pragma Singleton
pragma ComponentBehavior: Bound
import QtQuick
import Quickshell

Singleton {
	id: root

	// All state comes from DaemonSocket (daemon is single source of truth).
	readonly property bool tabletMode: DaemonSocket.hardwareTablet
	readonly property bool textInputActive: DaemonSocket.textFocus
	readonly property bool effectiveTabletMode: DaemonSocket.effectiveTabletMode
	readonly property string mode: DaemonSocket.userMode

	function cycleMode() { DaemonSocket.cycleMode() }
	function setMode(newMode) { DaemonSocket.setMode(newMode) }
}
