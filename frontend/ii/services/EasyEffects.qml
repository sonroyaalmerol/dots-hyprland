import qs.modules.common
import qs.services
import QtQuick
import Quickshell
pragma Singleton
pragma ComponentBehavior: Bound

/**
 * Handles EasyEffects active state and presets.
 */
Singleton {
	id: root

	property bool available: DaemonSocket.easyEffectsAvailable
	property bool active: DaemonSocket.easyEffectsActive

	function fetchAvailability() {}
	function fetchActiveState() {}

	function disable() { DaemonSocket.easyEffectsDisable() }
	function enable() { DaemonSocket.easyEffectsEnable() }
	function toggle() { DaemonSocket.easyEffectsToggle() }
}
