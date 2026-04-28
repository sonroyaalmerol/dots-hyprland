pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Io

/**
 * Central Unix socket connection to snry-daemon.
 * Owns the relay process and dispatches all events to consumers.
 *
 * Input: plain text commands (auth <pw>, lock, unlock, press, release, etc.)
 * Output: JSON events per line, dispatched via signals.
 */
Singleton {
	id: root

	// ── Connection state ────────────────────────────────────────────────
	property bool connected: false
	property bool powerSuspended: false

	// ── Signals for daemon events ───────────────────────────────────────
	signal lockStateChanged(bool locked)
	signal authResult(var data)
	signal lockoutTick(int remainingSeconds)
	signal powerStateChanged(bool suspended)
	signal tabletModeChanged(bool tablet)
	signal textFocusChanged(bool active)
	signal resourceDataUpdated(var data)
	signal hyprlandDataUpdated(var data)

	// ── Command senders ─────────────────────────────────────────────────
	function authenticate(password) { sendCommand("auth " + password) }
	function lock() { sendCommand("lock") }
	function unlock() { sendCommand("unlock") }

	function sendCommand(cmd) {
		if (daemonProc.running) {
			daemonProc.write(cmd + "\n")
		} else {
			console.warn("[DaemonSocket] Cannot send, process not running")
		}
	}

	// ── Reconnect timer ─────────────────────────────────────────────────
	Timer {
		id: reconnectTimer
		interval: 2000
		repeat: false
		onTriggered: {
			daemonProc.running = false
			daemonProc.running = true
		}
	}

	// ── Python relay process ────────────────────────────────────────────
	Process {
		id: daemonProc
		running: true

		command: ["python3", "-u", "-c", `
import socket, os, sys, select, time

sock_path = os.path.join(os.environ.get('XDG_RUNTIME_DIR', ''), 'snry-daemon.sock')

def relay(s):
	print('CONNECTED', flush=True)
	while True:
		rlist, _, _ = select.select([s, sys.stdin], [], [], 30)
		if s in rlist:
			data = s.recv(65536)
			if not data:
				return
			sys.stdout.buffer.write(data)
			sys.stdout.buffer.flush()
		if sys.stdin in rlist:
			line = sys.stdin.buffer.readline()
			if not line:
				return
			s.sendall(line)

while True:
	try:
		if os.path.exists(sock_path):
			s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
			s.settimeout(2)
			s.connect(sock_path)
			s.settimeout(None)
			print('CONNECTED', flush=True)
			relay(s)
			s.close()
	except Exception:
		pass
	time.sleep(2)
`]

		stdout: SplitParser {
			onRead: data => {
				if (data === "CONNECTED") {
					root.connected = true
					console.log("[DaemonSocket] connected to snry-daemon")
					return
				}
				try {
					const parsed = JSON.parse(data)
					root.dispatch(parsed)
				} catch (e) {
					console.warn("[DaemonSocket] parse error:", e, data)
				}
			}
		}

		onExited: (exitCode, exitStatus) => {
			root.connected = false
			reconnectTimer.start()
		}
	}

	// ── Event dispatcher ────────────────────────────────────────────────
	function dispatch(obj) {
		switch (obj.event) {
		case "lock_state":
			if (obj.data) root.lockStateChanged(obj.data.locked === true)
			break
		case "auth_result":
			if (obj.data) root.authResult(obj.data)
			break
		case "lockout_tick":
			if (obj.data) root.lockoutTick(obj.data.remainingSeconds || 0)
			break
		case "power_state":
			if (obj.data) {
				root.powerSuspended = obj.data.suspended === true
				root.powerStateChanged(obj.data.suspended === true)
			}
			break
		case "tablet_mode":
			root.tabletModeChanged(obj.active === true)
			break
		case "text_focus":
			root.textFocusChanged(obj.active === true)
			break
		case "resources":
			if (obj.data) root.resourceDataUpdated(obj.data)
			break
		case "hyprland_data":
			if (obj.data) root.hyprlandDataUpdated(obj.data)
			break
		}
	}
}
