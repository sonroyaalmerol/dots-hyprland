pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Io

Singleton {
	id: root

	// Consolidated state from daemon (single source of truth).
	property bool effectiveTabletMode: false
	property bool textFocus: false
	property bool hardwareTablet: false
	property string userMode: "auto"
	property bool oskVisible: false
	property bool oskDismissed: false
	property bool oskPinned: false
	property bool screenLocked: false

	// Resource metrics from daemon.
	property real cpuUsage: 0
	property real memoryTotal: 1
	property real memoryFree: 0
	property real memoryAvailable: 0
	property real memoryUsed: 0
	property real memoryUsedPercentage: 0
	property real swapTotal: 1
	property real swapFree: 0
	property real swapUsed: 0
	property real swapUsedPercentage: 0

	// Backward compat signals.
	signal lockStateChanged(bool locked)
	signal authResult(var data)
	signal lockoutTick(int remainingSeconds)
	signal autoscaleDone()
	signal autoscaleError(string error)
	signal checkdepsDone()
	signal checkdepsError(string error)
	signal diagnoseDone()
	signal diagnoseError(string error)

	property bool connected: daemonSocket.connected

	readonly property string socketPath: Quickshell.env("XDG_RUNTIME_DIR") + "/snry-daemon.sock"

	function authenticate(password) { sendCommand("auth " + password) }
	function lock() { sendCommand("lock") }
	function unlock() { sendCommand("unlock") }
	function lockStartup() { sendCommand("lock-startup") }

	// State commands.
	function setMode(mode) { sendCommand("set-mode " + mode) }
	function cycleMode() { sendCommand("cycle-mode") }
	function oskDismiss() { sendCommand("osk-dismiss") }
	function oskUndismiss() { sendCommand("osk-undismiss") }
	function oskToggle() { sendCommand("osk-toggle") }
	function oskShow() { sendCommand("osk-show") }
	function oskHide() { sendCommand("osk-hide") }
	function oskPin() { sendCommand("osk-pin") }
	function oskUnpin() { sendCommand("osk-unpin") }

	function sendCommand(cmd) {
		if (!daemonSocket.connected) {
			return
		}
		daemonSocket.write(cmd + "\n")
		daemonSocket.flush()
	}

	Socket {
		id: daemonSocket
		path: root.socketPath
		connected: true

		parser: SplitParser {
			splitMarker: "\n"
			onRead: data => {
				const line = data.trim()
				if (line.length === 0)
					return
				try {
					const obj = JSON.parse(line)
					root.dispatchEvent(obj)
				} catch (e) {}
			}
		}

		onError: error => {}
	}

	Timer {
		id: reconnectTimer
		interval: 3000
		repeat: false
		running: !daemonSocket.connected
		onTriggered: {
			daemonSocket.connected = false
			daemonSocket.connected = true
		}
	}

	function dispatchEvent(obj) {
		if (obj.event === "state" && obj.data) {
			// Consolidated state update.
			if (obj.data.effective_tablet_mode !== undefined)
				root.effectiveTabletMode = obj.data.effective_tablet_mode
			if (obj.data.text_focus !== undefined)
				root.textFocus = obj.data.text_focus
			if (obj.data.hardware_tablet !== undefined)
				root.hardwareTablet = obj.data.hardware_tablet
			if (obj.data.user_mode !== undefined)
				root.userMode = obj.data.user_mode
			if (obj.data.osk_visible !== undefined)
				root.oskVisible = obj.data.osk_visible
			if (obj.data.osk_dismissed !== undefined)
				root.oskDismissed = obj.data.osk_dismissed
			if (obj.data.osk_pinned !== undefined)
				root.oskPinned = obj.data.osk_pinned
			if (obj.data.screen_locked !== undefined)
				root.screenLocked = obj.data.screen_locked
		} else if (obj.event === "lock_state" && obj.data) {
			lockStateChanged(obj.data.locked === true)
		} else if (obj.event === "auth_result" && obj.data) {
			authResult(obj.data)
		} else if (obj.event === "lockout_tick" && obj.data) {
			lockoutTick(obj.data.remainingSeconds || 0)
		} else if (obj.event === "autoscale_done") {
			autoscaleDone()
		} else if (obj.event === "autoscale_error" && obj.data) {
			autoscaleError(obj.data.error || "unknown error")
		} else if (obj.event === "checkdeps_done") {
			checkdepsDone()
		} else if (obj.event === "checkdeps_error" && obj.data) {
			checkdepsError(obj.data.error || "unknown error")
		} else if (obj.event === "diagnose_done") {
			diagnoseDone()
		} else if (obj.event === "diagnose_error" && obj.data) {
			diagnoseError(obj.data.error || "unknown error")
		} else if (obj.event === "resources" && obj.data) {
			if (obj.data.cpuUsage !== undefined)
				root.cpuUsage = obj.data.cpuUsage
			if (obj.data.memoryTotal !== undefined)
				root.memoryTotal = obj.data.memoryTotal
			if (obj.data.memoryFree !== undefined)
				root.memoryFree = obj.data.memoryFree
			if (obj.data.memoryAvailable !== undefined) {
				root.memoryAvailable = obj.data.memoryAvailable
				root.memoryUsed = root.memoryTotal - root.memoryAvailable
				root.memoryUsedPercentage = root.memoryUsed / root.memoryTotal
			}
			if (obj.data.swapTotal !== undefined)
				root.swapTotal = obj.data.swapTotal
			if (obj.data.swapFree !== undefined) {
				root.swapFree = obj.data.swapFree
				root.swapUsed = root.swapTotal - root.swapFree
				root.swapUsedPercentage = root.swapTotal > 0 ? root.swapUsed / root.swapTotal : 0
			}
		}
	}
}
