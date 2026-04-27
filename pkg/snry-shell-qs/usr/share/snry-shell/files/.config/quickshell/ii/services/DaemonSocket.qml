pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Io

Singleton {
	id: root

	signal lockStateChanged(bool locked)
	signal authResult(var data)
	signal lockoutTick(int remainingSeconds)

	property bool connected: false

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

	function sendCommand(cmd) {
		if (!bridgeProc.running) {
			console.warn("[DaemonSocket] Cannot send command, bridge not running")
			return
		}
		bridgeProc.write(cmd + "\n")
	}

	Timer {
		id: reconnectTimer
		interval: 3000
		repeat: false
		onTriggered: {
			bridgeProc.running = false
			bridgeProc.running = true
		}
	}

	Process {
		id: bridgeProc
		running: true

		command: ["python3", "-u", "-c", `
import socket, os, sys, select, time

sock_path = os.path.join(os.environ.get('XDG_RUNTIME_DIR', ''), 'snry-daemon.sock')

def relay(s):
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
					return
				}
				try {
					const obj = JSON.parse(data)
					root.dispatchEvent(obj)
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

	function dispatchEvent(obj) {
		if (obj.event === "lock_state" && obj.data) {
			lockStateChanged(obj.data.locked === true)
		} else if (obj.event === "auth_result" && obj.data) {
			authResult(obj.data)
		} else if (obj.event === "lockout_tick" && obj.data) {
			lockoutTick(obj.data.remainingSeconds || 0)
		}
	}
}
