pragma Singleton

import qs.modules.common
import qs.modules.common.functions
import qs.services
import QtQuick
import Quickshell

Singleton {
	id: root

	property bool packageManagerRunning: DaemonSocket.sessionPackageManagerRunning
	property bool downloadRunning: DaemonSocket.sessionDownloadRunning

	function refresh() {
		// Daemon handles periodic checks. refresh() is a no-op.
	}
}
