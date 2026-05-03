pragma Singleton

import qs.modules.common
import qs.modules.common.functions
import qs.services
import QtQuick
import Quickshell

/*
 * System updates service. Currently only supports Arch.
 */
Singleton {
	id: root

	property bool available: false
	property bool checking: false
	property int count: 0

	readonly property bool updateAdvised: available && count > Config.options.updates.adviseUpdateThreshold
	readonly property bool updateStronglyAdvised: available && count > Config.options.updates.stronglyAdviseUpdateThreshold

	Connections {
		target: DaemonSocket

		function onUpdatesUpdated() {
			root.available = DaemonSocket.updatesAvailable
			root.count = DaemonSocket.updatesCount
		}
	}

	function load() {}
	function refresh() {}

	Component.onCompleted: {
		if (DaemonSocket.updatesAvailable) {
			onUpdatesUpdated()
		}
	}
}