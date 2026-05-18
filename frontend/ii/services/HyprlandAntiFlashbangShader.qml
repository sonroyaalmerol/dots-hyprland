pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.services
import qs.modules.common.models.hyprland

Singleton {
	id: root

	readonly property string shaderPath: Quickshell.shellPath("services/hyprlandAntiFlashbangShader/anti-flashbang.glsl")
	property bool enabled: confOpt.value == shaderPath

	function enable() {
		DaemonSocket.configSet("decoration:screen_shader", root.shaderPath)
		DaemonSocket.configSet("debug:damage_tracking", "1")
	}

	function disable() {
		DaemonSocket.configReset("decoration:screen_shader")
		DaemonSocket.configReset("debug:damage_tracking")
	}

	function toggle() {
		if (root.enabled) disable()
		else enable()
	}

	HyprlandConfigOption {
		id: confOpt
		key: "decoration:screen_shader"
	}
}