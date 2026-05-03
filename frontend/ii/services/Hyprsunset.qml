pragma Singleton

import QtQuick
import qs.modules.common
import qs.services
import Quickshell

/**
 * Simple hyprsunset service with automatic mode.
 * Routes actual hyprsunset commands through DaemonSocket.
 */
Singleton {
	id: root
	signal gammaChangeAttempt()

	readonly property real gammaLowerLimit: 25

	property string from: Config.options?.light?.night?.from ?? "19:00" 
	property string to: Config.options?.light?.night?.to ?? "06:30"
	property bool automatic: Config.options?.light?.night?.automatic && (Config?.ready ?? true)
	property int colorTemperature: Config.options?.light?.night?.colorTemperature ?? 5000
	property int gamma: DaemonSocket.hyprsunsetGamma
	property bool shouldBeOn
	property bool firstEvaluation: true
	property bool temperatureActive: DaemonSocket.hyprsunsetTemperatureActive

	property int fromHour: Number(from.split(":")[0])
	property int fromMinute: Number(from.split(":")[1])
	property int toHour: Number(to.split(":")[0])
	property int toMinute: Number(to.split(":")[1])

	property int clockHour: DateTime.clock.hours
	property int clockMinute: DateTime.clock.minutes

	property var manualActive
	property int manualActiveHour
	property int manualActiveMinute

	onClockMinuteChanged: reEvaluate()
	onAutomaticChanged: {
		root.manualActive = undefined;
		root.firstEvaluation = true;
		reEvaluate();
	}

	function inBetween(t, from, to) {
		if (from < to) {
			return (t >= from && t <= to);
		} else {
			// Wrapped around midnight
			return (t >= from || t <= to);
		}
	}

	function reEvaluate() {
		const t = clockHour * 60 + clockMinute;
		const from = fromHour * 60 + fromMinute;
		const to = toHour * 60 + toMinute;
		const manualActive = manualActiveHour * 60 + manualActiveMinute;

		if (root.manualActive !== undefined && (inBetween(from, manualActive, t) || inBetween(to, manualActive, t))) {
			root.manualActive = undefined;
		}
		root.shouldBeOn = inBetween(t, from, to);
		if (firstEvaluation) {
			firstEvaluation = false;
			root.ensureState();
		}
	}

	onShouldBeOnChanged: ensureState()
	function ensureState() {
		if (!root.automatic || root.manualActive !== undefined)
			return;
		if (root.shouldBeOn) {
			root.enableTemperature();
		} else {
			root.disableTemperature();
		}
	}

	function load() {
		root.ensureState();
	}

	function enableTemperature() {
		DaemonSocket.hyprsunsetEnableTemperature();
	}

	function disableTemperature() {
		DaemonSocket.hyprsunsetDisableTemperature();
	}

	function setGamma(gamma) {
		DaemonSocket.hyprsunsetSetGamma(Math.max(root.gammaLowerLimit, Math.min(100, gamma)));
		root.gammaChangeAttempt();
	}

	function fetchState() {
		// Daemon handles state. No-op.
	}

	function toggleTemperature(active = undefined) {
		if (root.manualActive === undefined) {
			root.manualActive = root.temperatureActive;
			root.manualActiveHour = root.clockHour;
			root.manualActiveMinute = root.clockMinute;
		}

		root.manualActive = active !== undefined ? active : !root.manualActive;
		DaemonSocket.hyprsunsetToggleTemperature(root.manualActive);
	}

	Connections {
		target: Config.options.light.night
		function onColorTemperatureChanged() {
			if (!root.temperatureActive) return;
			DaemonSocket.hyprsunsetEnableTemperature();
		}
	}
}
