pragma Singleton
pragma ComponentBehavior: Bound

import qs.modules.common
import QtQuick
import Quickshell

Singleton {
	id: root

	property real memoryTotal: DaemonSocket.memoryTotal
	property real memoryFree: DaemonSocket.memoryFree
	property real memoryUsed: DaemonSocket.memoryUsed
	property real memoryUsedPercentage: DaemonSocket.memoryUsedPercentage
	property real swapTotal: DaemonSocket.swapTotal
	property real swapFree: DaemonSocket.swapFree
	property real swapUsed: DaemonSocket.swapUsed
	property real swapUsedPercentage: DaemonSocket.swapUsedPercentage
	property real cpuUsage: DaemonSocket.cpuUsage

	property string maxAvailableMemoryString: kbToGbString(memoryTotal)
	property string maxAvailableSwapString: kbToGbString(swapTotal)
	property string maxAvailableCpuString: "--"

	readonly property int historyLength: Config?.options?.resources?.historyLength ?? 60
	property list<real> cpuUsageHistory: []
	property list<real> memoryUsageHistory: []
	property list<real> swapUsageHistory: []

	function kbToGbString(kb) {
		return (kb / (1024 * 1024)).toFixed(1) + " GB"
	}

	function updateMemoryUsageHistory() {
		memoryUsageHistory = [...memoryUsageHistory, memoryUsedPercentage]
		if (memoryUsageHistory.length > historyLength) {
			memoryUsageHistory.shift()
		}
	}

	function updateSwapUsageHistory() {
		swapUsageHistory = [...swapUsageHistory, swapUsedPercentage]
		if (swapUsageHistory.length > historyLength) {
			swapUsageHistory.shift()
		}
	}

	function updateCpuUsageHistory() {
		cpuUsageHistory = [...cpuUsageHistory, cpuUsage]
		if (cpuUsageHistory.length > historyLength) {
			cpuUsageHistory.shift()
		}
	}

	function updateHistories() {
		updateMemoryUsageHistory()
		updateSwapUsageHistory()
		updateCpuUsageHistory()
	}

	Timer {
		interval: Config?.options?.resources?.updateInterval ?? 3000
		running: true
		repeat: true
		onTriggered: {
			root.updateHistories()
		}
	}
}