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
		}
	}
}
