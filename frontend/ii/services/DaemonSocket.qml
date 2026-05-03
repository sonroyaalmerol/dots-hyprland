pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Io

Singleton {
	id: root

	// Consolidated state from daemon (single source of truth).
	property bool effectiveTabletMode: false
	property bool textFocus: false
	property bool hardwareTablet: false
	property string userMode: "auto"
	property bool oskVisible: false
	property bool oskDismissed: false
	property bool oskPinned: false
	property bool screenLocked: false

	// Resource metrics from daemon.
	property real cpuUsage: 0
	property real memoryTotal: 1
	property real memoryFree: 0
	property real memoryAvailable: 0
	property real memoryUsed: 0
	property real memoryUsedPercentage: 0
	property real swapTotal: 1
	property real swapFree: 0
	property real swapUsed: 0
	property real swapUsedPercentage: 0

	// Hyprland data from daemon.
	property var hyprWindows: []
	property var hyprMonitors: []
	property var hyprWorkspaces: []
	property var hyprLayers: ({})
	property var hyprActiveWorkspace: null

	// Weather data from daemon.
	property string weatherRaw: ""
	property string weatherCity: ""

	// Updates data from daemon.
	property bool updatesAvailable: false
	property int updatesCount: 0

	// System info from daemon.
	property string systemDistroName: "Unknown"
	property string systemDistroId: "unknown"
	property string systemDistroIcon: "linux-symbolic"
	property string systemUsername: "user"
	property string systemHomeUrl: ""
	property string systemDocumentationUrl: ""
	property string systemSupportUrl: ""
	property string systemBugReportUrl: ""
	property string systemPrivacyPolicyUrl: ""
	property string systemLogo: ""
	property string systemDesktopEnvironment: ""
	property string systemWindowingSystem: ""

	// Session warnings from daemon.
	property bool sessionPackageManagerRunning: false
	property bool sessionDownloadRunning: false

	// EasyEffects from daemon.
	property bool easyEffectsAvailable: false
	property bool easyEffectsActive: false

	// Hyprland keybinds from daemon.
	property string hyprDefaultKeybinds: '{"children":[]}'
	property string hyprUserKeybinds: '{"children":[]}'

	// Hyprland XKB from daemon.
	property string hyprCurrentLayoutName: ""
	property string hyprCurrentLayoutCode: ""
	property var hyprLayoutCodes: []

	// Cliphist from daemon.
	property var cliphistEntries: []

	// Hyprsunset from daemon.
	property bool hyprsunsetTemperatureActive: false
	property int hyprsunsetGamma: 100

	// Brightness from daemon.
	property var brightnessMonitors: ({})

	// Warp and GameMode from daemon.
	property bool warpInstalled: false
	property bool warpConnected: false
	property string warpStatus: ""
	property bool gameModeEnabled: false

	// Network from daemon.
	property bool networkWifiEnabled: false
	property string networkWifiStatus: "disconnected"
	property bool networkEthernet: false
	property bool networkWifi: false
	property string networkName: ""
	property int networkStrength: 0
	property var networkWifiNetworks: []

	// Backward compat signals.
	signal lockStateChanged(bool locked)
	signal authResult(var data)
	signal lockoutTick(int remainingSeconds)
	signal autoscaleDone()
	signal autoscaleError(string error)
	signal checkdepsDone()
	signal checkdepsError(string error)
	signal diagnoseDone()
	signal diagnoseError(string error)
	signal weatherUpdated()
	signal updatesUpdated()
	signal systemInfoUpdated()
	signal sessionWarningsUpdated()
	signal easyEffectsUpdated()
	signal hyprKeybindsUpdated()
	signal hyprXkbUpdated()
	signal cliphistUpdated()
	signal hyprsunsetUpdated()
	signal brightnessUpdated()
	signal networkUpdated()
	signal networkConnectResult(var data)

	// Warp and GameMode signals.
	signal warpStatusUpdated()
	signal gameModeUpdated()
	signal conflictResult(var trays, var notifications)
	signal hyprconfigValue(string key, string value)

	property bool connected: daemonSocket.connected

	readonly property string socketPath: Quickshell.env("XDG_RUNTIME_DIR") + "/snry-daemon.sock"

	function authenticate(password) { sendCommand("auth " + password) }
	function lock() { sendCommand("lock") }
	function unlock() { sendCommand("unlock") }
	function lockStartup() { sendCommand("lock-startup") }

	// State commands.
	function setMode(mode) { sendCommand("set-mode " + mode) }
	function cycleMode() { sendCommand("cycle-mode") }
	function oskDismiss() { sendCommand("osk-dismiss") }
	function oskUndismiss() { sendCommand("osk-undismiss") }
	function oskToggle() { sendCommand("osk-toggle") }
	function oskShow() { sendCommand("osk-show") }
	function oskHide() { sendCommand("osk-hide") }
	function oskPin() { sendCommand("osk-pin") }
	function oskUnpin() { sendCommand("osk-unpin") }

	function sendCommand(cmd) {
		if (!daemonSocket.connected) {
			return
		}
		daemonSocket.write(cmd + "\n")
		daemonSocket.flush()
	}

	function easyEffectsToggle() { sendCommand("easyeffects-toggle") }
	function easyEffectsEnable() { sendCommand("easyeffects-enable") }
	function easyEffectsDisable() { sendCommand("easyeffects-disable") }
	function hyprsunsetSetGamma(gamma) { sendCommand("hyprsunset-gamma " + gamma) }
	function hyprsunsetEnableTemperature() { sendCommand("hyprsunset-enable") }
	function hyprsunsetDisableTemperature() { sendCommand("hyprsunset-disable") }
	function hyprsunsetToggleTemperature(active) { sendCommand("hyprsunset-toggle " + (active !== undefined ? active : "")) }
	function brightnessSet(screen, value) { sendCommand("brightness-set " + screen + " " + value) }
	function brightnessIncrement(screen, delta) { sendCommand("brightness-increment " + screen + " " + delta) }
	function brightnessGet(screen) { sendCommand("brightness-get " + screen) }
	function wifiEnable() { sendCommand("wifi-enable") }
	function wifiDisable() { sendCommand("wifi-disable") }
	function wifiToggle() { sendCommand("wifi-toggle") }
	function wifiRescan() { sendCommand("wifi-rescan") }
	function wifiConnect(ssid) { sendCommand("wifi-connect " + ssid) }
	function wifiDisconnect(ssid) { sendCommand("wifi-disconnect " + ssid) }
	function wifiChangePassword(ssid, password) { sendCommand("wifi-change-password " + ssid + " " + password) }
	function cliphistRefresh() { sendCommand("cliphist-list") }
	function cliphistDelete(entry) { sendCommand("cliphist-delete " + entry) }
	function cliphistWipe() { sendCommand("cliphist-wipe") }

	function warpConnect() { sendCommand("warp-connect") }
	function warpDisconnect() { sendCommand("warp-disconnect") }
	function warpToggle() { sendCommand("warp-toggle") }
	function warpRegister() { sendCommand("warp-register") }
	function gameModeEnable() { sendCommand("gamemode-enable") }
	function gameModeDisable() { sendCommand("gamemode-disable") }
	function gameModeToggle() { sendCommand("gamemode-toggle") }
	function conflictCheck() { sendCommand("conflict-check") }
	function fpsSet(value) { sendCommand("fps-set " + value) }
	function hyprconfigGet(key) { sendCommand("hyprconfig-get " + key) }
	function hyprconfigSet(key, value) { sendCommand("hyprconfig-set " + key + " " + value) }
	function hyprconfigReset(key) { sendCommand("hyprconfig-reset " + key) }

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
				} catch (e) {}
			}
		}

		onError: error => {}
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
		if (obj.event === "state" && obj.data) {
			// Consolidated state update.
			if (obj.data.effective_tablet_mode !== undefined)
				root.effectiveTabletMode = obj.data.effective_tablet_mode
			if (obj.data.text_focus !== undefined)
				root.textFocus = obj.data.text_focus
			if (obj.data.hardware_tablet !== undefined)
				root.hardwareTablet = obj.data.hardware_tablet
			if (obj.data.user_mode !== undefined)
				root.userMode = obj.data.user_mode
			if (obj.data.osk_visible !== undefined)
				root.oskVisible = obj.data.osk_visible
			if (obj.data.osk_dismissed !== undefined)
				root.oskDismissed = obj.data.osk_dismissed
			if (obj.data.osk_pinned !== undefined)
				root.oskPinned = obj.data.osk_pinned
			if (obj.data.screen_locked !== undefined)
				root.screenLocked = obj.data.screen_locked
		} else if (obj.event === "lock_state" && obj.data) {
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
		} else if (obj.event === "resources" && obj.data) {
			if (obj.data.cpuUsage !== undefined)
				root.cpuUsage = obj.data.cpuUsage
			if (obj.data.memoryTotal !== undefined)
				root.memoryTotal = obj.data.memoryTotal
			if (obj.data.memoryFree !== undefined)
				root.memoryFree = obj.data.memoryFree
			if (obj.data.memoryAvailable !== undefined) {
				root.memoryAvailable = obj.data.memoryAvailable
				root.memoryUsed = root.memoryTotal - root.memoryAvailable
				root.memoryUsedPercentage = root.memoryUsed / root.memoryTotal
			}
			if (obj.data.swapTotal !== undefined)
				root.swapTotal = obj.data.swapTotal
			if (obj.data.swapFree !== undefined) {
				root.swapFree = obj.data.swapFree
				root.swapUsed = root.swapTotal - root.swapFree
				root.swapUsedPercentage = root.swapTotal > 0 ? root.swapUsed / root.swapTotal : 0
			}
		} else if (obj.event === "hyprland_data" && obj.data) {
			root.hyprWindows = obj.data.windows || []
			root.hyprMonitors = obj.data.monitors || []
			root.hyprWorkspaces = obj.data.workspaces || []
			root.hyprLayers = obj.data.layers || {}
			root.hyprActiveWorkspace = obj.data.activeWorkspace || null
		} else if (obj.event === "weather" && obj.data) {
			root.weatherRaw = obj.data.raw || ""
			root.weatherCity = obj.data.city || ""
			weatherUpdated()
		} else if (obj.event === "updates" && obj.data) {
			root.updatesAvailable = obj.data.available || false
			root.updatesCount = obj.data.count || 0
			updatesUpdated()
		} else if (obj.event === "system_info" && obj.data) {
			if (obj.data.distroName !== undefined) root.systemDistroName = obj.data.distroName
			if (obj.data.distroId !== undefined) root.systemDistroId = obj.data.distroId
			if (obj.data.distroIcon !== undefined) root.systemDistroIcon = obj.data.distroIcon
			if (obj.data.username !== undefined) root.systemUsername = obj.data.username
			if (obj.data.homeUrl !== undefined) root.systemHomeUrl = obj.data.homeUrl
			if (obj.data.documentationUrl !== undefined) root.systemDocumentationUrl = obj.data.documentationUrl
			if (obj.data.supportUrl !== undefined) root.systemSupportUrl = obj.data.supportUrl
			if (obj.data.bugReportUrl !== undefined) root.systemBugReportUrl = obj.data.bugReportUrl
			if (obj.data.privacyPolicyUrl !== undefined) root.systemPrivacyPolicyUrl = obj.data.privacyPolicyUrl
			if (obj.data.logo !== undefined) root.systemLogo = obj.data.logo
			if (obj.data.desktopEnvironment !== undefined) root.systemDesktopEnvironment = obj.data.desktopEnvironment
			if (obj.data.windowingSystem !== undefined) root.systemWindowingSystem = obj.data.windowingSystem
			systemInfoUpdated()
		} else if (obj.event === "session_warnings" && obj.data) {
			if (obj.data.packageManagerRunning !== undefined) root.sessionPackageManagerRunning = obj.data.packageManagerRunning
			if (obj.data.downloadRunning !== undefined) root.sessionDownloadRunning = obj.data.downloadRunning
			sessionWarningsUpdated()
		} else if (obj.event === "easyeffects" && obj.data) {
			if (obj.data.available !== undefined) root.easyEffectsAvailable = obj.data.available
			if (obj.data.active !== undefined) root.easyEffectsActive = obj.data.active
			easyEffectsUpdated()
		} else if (obj.event === "hypr_keybinds" && obj.data) {
			if (obj.data.default !== undefined) root.hyprDefaultKeybinds = obj.data.default
			if (obj.data.user !== undefined) root.hyprUserKeybinds = obj.data.user
			hyprKeybindsUpdated()
		} else if (obj.event === "hypr_xkb" && obj.data) {
			if (obj.data.layoutCodes !== undefined) root.hyprLayoutCodes = obj.data.layoutCodes
			if (obj.data.currentLayoutName !== undefined) root.hyprCurrentLayoutName = obj.data.currentLayoutName
			if (obj.data.currentLayoutCode !== undefined) root.hyprCurrentLayoutCode = obj.data.currentLayoutCode
			hyprXkbUpdated()
		} else if (obj.event === "hyprsunset" && obj.data) {
			if (obj.data.temperatureActive !== undefined) root.hyprsunsetTemperatureActive = obj.data.temperatureActive
			if (obj.data.gamma !== undefined) root.hyprsunsetGamma = obj.data.gamma
			hyprsunsetUpdated()
		} else if (obj.event === "brightness" && obj.data) {
			root.brightnessMonitors = obj.data
			brightnessUpdated()
		} else if (obj.event === "brightness_value" && obj.data) {
			brightnessUpdated()
		} else if (obj.event === "network" && obj.data) {
			if (obj.data.wifiEnabled !== undefined) root.networkWifiEnabled = obj.data.wifiEnabled
			if (obj.data.wifiStatus !== undefined) root.networkWifiStatus = obj.data.wifiStatus
			if (obj.data.ethernet !== undefined) root.networkEthernet = obj.data.ethernet
			if (obj.data.wifi !== undefined) root.networkWifi = obj.data.wifi
			if (obj.data.networkName !== undefined) root.networkName = obj.data.networkName
			if (obj.data.networkStrength !== undefined) root.networkStrength = obj.data.networkStrength
			if (obj.data.wifiNetworks !== undefined) root.networkWifiNetworks = obj.data.wifiNetworks
			networkUpdated()
		} else if (obj.event === "cliphist_list" && obj.data) {
			if (obj.data.entries !== undefined) root.cliphistEntries = obj.data.entries
			cliphistUpdated()
		} else if (obj.event === "network_connect_result" && obj.data) {
			networkConnectResult(obj.data)
		} else if (obj.event === "warp_status" && obj.data) {
			root.warpInstalled = obj.data.installed ?? false
			root.warpConnected = obj.data.connected ?? false
			root.warpStatus = obj.data.status ?? ""
			warpStatusUpdated()
		} else if (obj.event === "game_mode" && obj.data) {
			root.gameModeEnabled = obj.data.enabled ?? false
			gameModeUpdated()
		} else if (obj.event === "conflict_result" && obj.data) {
			conflictResult(obj.data.trays ?? [], obj.data.notifications ?? [])
		} else if (obj.event === "hyprconfig_value" && obj.data) {
			hyprconfigValue(obj.data.key ?? "", obj.data.value ?? "")
		}
	}
}
