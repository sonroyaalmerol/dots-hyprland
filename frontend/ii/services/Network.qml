pragma Singleton
pragma ComponentBehavior: Bound

import qs.services
import qs.services.network
import QtQuick
import Quickshell

/**
 * Network service. Reads all state from daemon, sends commands via DaemonSocket.
 */
Singleton {
	id: root

	property bool wifi: DaemonSocket.networkWifi
	property bool ethernet: DaemonSocket.networkEthernet
	property bool wifiEnabled: DaemonSocket.networkWifiEnabled
	property bool wifiScanning: false
	property bool wifiConnecting: false
	property string wifiStatus: DaemonSocket.networkWifiStatus
	property string networkName: DaemonSocket.networkName
	property int networkStrength: DaemonSocket.networkStrength
	property WifiAccessPoint wifiConnectTarget

	// Build WifiAccessPoint objects from daemon data
	property var rawNetworks: DaemonSocket.networkWifiNetworks
	readonly property list<WifiAccessPoint> wifiNetworks: buildNetworks(rawNetworks)
	readonly property WifiAccessPoint active: wifiNetworks.find(n => n.active) ?? null
	readonly property list<var> friendlyWifiNetworks: [...wifiNetworks].sort((a, b) => {
		if (a.active && !b.active)
			return -1;
		if (!a.active && b.active)
			return 1;
		return b.strength - a.strength;
	})

	readonly property string materialSymbol: root.ethernet
		? "lan"
		: (root.wifiEnabled && root.wifiStatus === "connected")
			? (
				(root.active?.strength ?? 0) > 83 ? "signal_wifi_4_bar" :
				(root.active?.strength ?? 0) > 67 ? "network_wifi" :
				(root.active?.strength ?? 0) > 50 ? "network_wifi_3_bar" :
				(root.active?.strength ?? 0) > 33 ? "network_wifi_2_bar" :
				(root.active?.strength ?? 0) > 17 ? "network_wifi_1_bar" :
				"signal_wifi_0_bar"
			)
			: (root.wifiStatus === "connecting")
				? "signal_wifi_statusbar_not_connected"
				: (root.wifiStatus === "disconnected")
					? "wifi_find"
					: (root.wifiStatus === "disabled")
						? "signal_wifi_off"
						: "signal_wifi_bad"

	function buildNetworks(raw) {
		if (!raw || raw.length === 0) return []
		var result = []
		for (var i = 0; i < raw.length; i++) {
			result.push(apComp.createObject(root, {
				lastIpcObject: raw[i]
			}))
		}
		return result
	}

	// Control — route through DaemonSocket
	function enableWifi(enabled) {
		if (enabled) DaemonSocket.wifiEnable()
		else DaemonSocket.wifiDisable()
	}

	function toggleWifi() {
		DaemonSocket.wifiToggle()
	}

	function rescanWifi() {
		wifiScanning = true
		DaemonSocket.wifiRescan()
		// Scanning is fast, reset flag after a delay
		rescanTimer.start()
	}

	function connectToWifiNetwork(accessPoint) {
		accessPoint.askingPassword = false
		root.wifiConnectTarget = accessPoint
		wifiConnecting = true
		DaemonSocket.wifiConnect(accessPoint.ssid)
	}

	function disconnectWifiNetwork() {
		if (active) DaemonSocket.wifiDisconnect(active.ssid)
	}

	function openPublicWifiPortal() {
		Quickshell.execDetached(["xdg-open", "https://nmcheck.gnome.org/"])
	}

	function changePassword(network, password) {
		network.askingPassword = false
		DaemonSocket.wifiChangePassword(network.ssid, password)
	}

	Timer {
		id: rescanTimer
		interval: 3000
		onTriggered: root.wifiScanning = false
	}

	Connections {
		target: DaemonSocket

		function onNetworkConnectResult(data) {
			root.wifiConnecting = false
			if (data.askingPassword && root.wifiConnectTarget) {
				root.wifiConnectTarget.askingPassword = true
			}
			if (!data.success) {
				root.wifiConnectTarget = null
			}
		}
	}

	Component {
		id: apComp
		WifiAccessPoint {}
	}
}
