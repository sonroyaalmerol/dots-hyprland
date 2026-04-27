pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell

Singleton {
	id: root

	signal lockStateChanged(bool locked)
	signal authResult(var data)
	signal lockoutTick(int remainingSeconds)

	property bool connected: false

	function authenticate(password) {
		sendCommand("auth " + password)
	}

	function lock() {
		sendCommand("lock")
	}

	function unlock() {
		sendCommand("unlock")
	}

	property var _sendFn: null

	function sendCommand(cmd) {
		if (_sendFn) _sendFn(cmd + "\n")
	}

	// Forward lock events from TabletMode's unified relay.
	// Connected lazily via Component.onCompleted to avoid circular init.
	Component.onCompleted: {
		Qt.callLater(() => {
			if (typeof TabletMode !== "undefined" && TabletMode.daemonSocket) {
				root._sendFn = (data) => { TabletMode.daemonSocket.write(data) }
				root.connected = Qt.binding(() => TabletMode.daemonConnected)
			}
		})
	}

	Connections {
		target: typeof TabletMode !== "undefined" ? TabletMode : null
		function onLockStateChanged(locked) { root.lockStateChanged(locked) }
		function onAuthResult(data) { root.authResult(data) }
		function onLockoutTick(remainingSeconds) { root.lockoutTick(remainingSeconds) }
	}
}
