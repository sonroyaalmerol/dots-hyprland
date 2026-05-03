pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Io

/**
 * Persistent Unix socket connection to snry-daemon.
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

	property bool connected: false
	property string accumulatedText: ""

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
		if (!daemonProc.running) {
			console.warn("[DaemonSocket] Cannot send command, process not running")
			return
		}
		daemonProc.write(cmd + "\n")
	}

	Timer {
		id: reconnectTimer
		interval: 2000
		repeat: false
		onTriggered: {
			daemonProc.running = false
			daemonProc.running = true
		}
	}

	Process {
		id: daemonProc
		running: true
		command: ["socat", "-,ignoreeof", "UNIX-CONNECT:" + root.socketPath]

		stdout: StdioCollector {
			id: stdoutCollector
			onStreamFinished: {
				root.connected = false
				reconnectTimer.start()
			}
		}

		onExited: (exitCode, exitStatus) => {
			root.connected = false
			reconnectTimer.start()
		}
	}

	// Poll accumulated stdout and parse JSON lines
	Timer {
		interval: 100
		repeat: true
		running: true
		onTriggered: root.parseOutput()
	}

	function parseOutput() {
		const raw = stdoutCollector.text
		if (raw.length === 0)
			return

		const lines = raw.split("\n")
		for (let i = 0; i < lines.length - 1; i++) {
			const line = lines[i].trim()
			if (line.length === 0)
				continue
			try {
				const obj = JSON.parse(line)
				dispatchEvent(obj)
			} catch (e) {
				console.warn("[DaemonSocket] parse error:", e, line)
			}
		}
		// Keep incomplete last line
		stdoutCollector.text = lines[lines.length - 1]
	}

	function dispatchEvent(obj) {
		if (obj.event === "lock_state" && obj.data) {
			root.connected = true
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
