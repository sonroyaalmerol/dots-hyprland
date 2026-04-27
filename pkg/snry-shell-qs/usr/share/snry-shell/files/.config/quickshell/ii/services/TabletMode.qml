pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Io

Singleton {
	id: root

	property bool tabletMode: false
	property bool textInputActive: false

	// Tri-state mode: "auto" | "tablet" | "desktop"
	property string mode: "auto"
	property bool effectiveTabletMode: {
		if (mode === "tablet") return true
		if (mode === "desktop") return false
		return tabletMode
	}
	property bool watcherRunning: daemonProc.running

	// Shared daemon connection state
	property bool daemonConnected: false

	function cycleMode() {
		if (root.mode === "auto") root.mode = "tablet"
		else if (root.mode === "tablet") root.mode = "desktop"
		else root.mode = "auto"
	}

	// Signals for lockscreen events forwarded from daemon
	signal lockStateChanged(bool locked)
	signal authResult(var data)
	signal lockoutTick(int remainingSeconds)

	// Wrapper exposing .connected and .write() for Ydotool compatibility
	property alias daemonSocket: daemonBridge
	QtObject {
		id: daemonBridge
		property bool connected: daemonProc.running
		function write(data) {
			daemonProc.write(data)
		}
	}

	// Python relay process: single persistent connection to snry-daemon
	// Handles bidirectional communication for ALL event types:
	//   daemon socket → stdout (JSON events dispatched by SplitParser)
	//   stdin → daemon socket (commands from DaemonSocket, Ydotool, etc.)
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
					root.daemonConnected = true
					console.log("[TabletMode] connected to snry-daemon")
					return
				}
				try {
					const parsed = JSON.parse(data)
					switch (parsed.event) {
					case "tablet_mode":
						root.tabletMode = parsed.active === true
						break
					case "text_focus":
						root.textInputActive = parsed.active === true
						break
					case "lock_state":
						if (parsed.data) root.lockStateChanged(parsed.data.locked === true)
						break
					case "auth_result":
						if (parsed.data) root.authResult(parsed.data)
						break
					case "lockout_tick":
						if (parsed.data) root.lockoutTick(parsed.data.remainingSeconds || 0)
						break
					}
				} catch (e) {
					console.warn("[TabletMode] parse error:", e, data)
				}
			}
		}

		onExited: (exitCode, exitStatus) => {
			root.daemonConnected = false
		}
	}
}
