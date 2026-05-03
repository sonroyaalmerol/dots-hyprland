pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Io

/**
 * Persistent Unix socket connection to snry-daemon using native Quickshell.Io.Socket.
 *
 * Input protocol (to daemon): plain text commands, one per line.
 *   auth <password>
 *   lock
 *   unlock
 *
 * Output protocol (from daemon): JSON objects, one per line.
 *   {"event":"lock_state","data":{"locked":true}}
 *   {"event":"auth_result","data":{"success":false,"remaining":2,"lockedOut":false,"message":"..."}}
 *   {"event":"lockout_tick","data":{"remainingSeconds":15}}
 */
Singleton {
	id: root

	signal lockStateChanged(bool locked)
	signal authResult(var data)
	signal lockoutTick(int remainingSeconds)
	signal autoscaleDone()
	signal autoscaleError(string error)
	signal checkdepsDone()
	signal checkdepsError(string error)
	signal diagnoseDone()
	signal diagnoseError(string error)
	signal tabletMode(bool active)
	signal textInputFocus(bool active)

	property bool connected: daemonSocket.connected

	readonly property string socketPath: Quickshell.env("XDG_RUNTIME_DIR") + "/snry-daemon.sock"

	function authenticate(password) {
		sendCommand("auth " + password)
	}

	function lock() {
		sendCommand("lock")
	}

	function unlock() {
		sendCommand("unlock")
	}

	function lockStartup() {
		sendCommand("lock-startup")
	}

	function sendCommand(cmd) {
		if (!daemonSocket.connected) {
			console.warn("[DaemonSocket] Cannot send command, not connected")
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
				} catch (e) {
					console.warn("[DaemonSocket] parse error:", e, line)
				}
			}
		}

		onError: error => {
			console.warn("[DaemonSocket] socket error:", error)
		}
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
		if (obj.event === "lock_state" && obj.data) {
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
		} else if (obj.event === "tablet_mode" && obj.data) {
			tabletMode(obj.data.active === true)
		} else if (obj.event === "text_focus" && obj.data) {
			textInputFocus(obj.data.active === true)
		}
	}
}
