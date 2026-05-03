pragma Singleton
pragma ComponentBehavior: Bound

import qs.modules.common
import qs.modules.common.functions
import qs.services
import Quickshell
import Quickshell.Hyprland
import Quickshell.Io
import QtQuick

Singleton {
	id: root
	signal brightnessChanged()

	readonly property list<BrightnessMonitor> monitors: Quickshell.screens.map(screen => monitorComp.createObject(root, {
		screen
	}))

	function getMonitorForScreen(screen): var {
		return monitors.find(m => m.screen === screen);
	}

	function increaseBrightness(): void {
		// if gamma is not yet 100, first increase gamma
		if (DaemonSocket.hyprsunsetGamma !== 100) {
			DaemonSocket.hyprsunsetSetGamma(DaemonSocket.hyprsunsetGamma + 5);
			return;
		}

		const focusedName = Hyprland.focusedMonitor.name;
		const monitor = monitors.find(m => focusedName === m.screen.name);
		if (monitor)
			monitor.setBrightness(monitor.brightness + 0.05);
	}

	function decreaseBrightness(): void {
		const focusedName = Hyprland.focusedMonitor.name;
		const monitor = monitors.find(m => focusedName === m.screen.name);
		if (monitor && monitor.brightness > 0)
			monitor.setBrightness(monitor.brightness - 0.05);
		// if brightness is 0, then decrease gamma
		else {
			DaemonSocket.hyprsunsetSetGamma(DaemonSocket.hyprsunsetGamma - 5);
		}
	}

	reloadableId: "brightness"

	component BrightnessMonitor: QtObject {
		id: monitor

		required property ShellScreen screen
		property bool isDDC: false
		property int rawMaxBrightness: 100
		property real brightness: 0.5
		property real brightnessMultiplier: 1.0
		property real multipliedBrightness: Math.max(0, Math.min(1, brightness * (Config.options.light.antiFlashbang.enable ? brightnessMultiplier : 1)))
		property bool ready: false
		property bool animateChanges: !monitor.isDDC

		onBrightnessChanged: {
			if (!monitor.ready) return;
			root.brightnessChanged();
		}

		Behavior on multipliedBrightness {
			enabled: monitor.animateChanges
			NumberAnimation {
				duration: 200
				easing.type: Easing.BezierSpline
				easing.bezierCurve: Appearance.animationCurves.expressiveEffects
			}
		}
		onMultipliedBrightnessChanged: {
			if (monitor.animateChanges) syncBrightness();
			else setTimer.restart();
		}

		property var daemonConn: Connections {
			target: DaemonSocket
			function onBrightnessUpdated() {
				const data = DaemonSocket.brightnessMonitors[monitor.screen.name];
				if (data) {
					monitor.isDDC = data.isDDC ?? false;
					monitor.rawMaxBrightness = data.maxRaw ?? 100;
					monitor.brightnessMultiplier = data.multiplier ?? 1.0;
					if (!monitor.ready) {
						monitor.brightness = data.brightness ?? 0.5;
					}
					monitor.ready = true;
				}
			}
		}

		// We need a delay for DDC monitors because they can be quite slow and might act weird with rapid changes
		property var setTimer: Timer {
			id: setTimer
			interval: monitor.isDDC ? 300 : 0
			onTriggered: syncBrightness();
		}

		function syncBrightness() {
			const brightnessValue = Math.max(monitor.multipliedBrightness, 0);
			DaemonSocket.brightnessSet(monitor.screen.name, brightnessValue);
		}

		function setBrightness(value: real): void {
			value = Math.max(0, Math.min(1, value));
			monitor.brightness = value;
		}

		function setBrightnessMultiplier(value: real): void {
			monitor.brightnessMultiplier = value;
		}
	}

	Component {
		id: monitorComp

		BrightnessMonitor {}
	}

	// External trigger points

	IpcHandler {
		target: "brightness"

		function increment() { root.increaseBrightness() }
		function decrement() { root.decreaseBrightness() }
	}

	GlobalShortcut {
		name: "brightnessIncrease"
		description: "Increase brightness"
		onPressed: root.increaseBrightness()
	}

	GlobalShortcut {
		name: "brightnessDecrease"
		description: "Decrease brightness"
		onPressed: root.decreaseBrightness()
	}
}
