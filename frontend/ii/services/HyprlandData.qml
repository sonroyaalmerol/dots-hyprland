pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Hyprland

/**
 * Provides access to some Hyprland data not available in Quickshell.Hyprland.
 */
Singleton {
	id: root
	property var windowList: []
	property var addresses: []
	property var windowByAddress: ({})
	property var workspaces: []
	property var workspaceIds: []
	property var workspaceById: ({})
	property var activeWorkspace: null
	property var monitors: []
	property var layers: ({})

	Connections {
		target: DaemonSocket

		function onHyprWindowsChanged() {
			root.windowList = DaemonSocket.hyprWindows
			let tempWinByAddress = {}
			for (let i = 0; i < root.windowList.length; ++i) {
				let win = root.windowList[i]
				tempWinByAddress[win.address] = win
			}
			root.windowByAddress = tempWinByAddress
			root.addresses = root.windowList.map(win => win.address)
		}

		function onHyprMonitorsChanged() {
			root.monitors = DaemonSocket.hyprMonitors
		}

		function onHyprWorkspacesChanged() {
			let rawWorkspaces = DaemonSocket.hyprWorkspaces
			root.workspaces = rawWorkspaces.filter(ws => ws.id >= 1 && ws.id <= 100)
			let tempWorkspaceById = {}
			for (let i = 0; i < root.workspaces.length; ++i) {
				let ws = root.workspaces[i]
				tempWorkspaceById[ws.id] = ws
			}
			root.workspaceById = tempWorkspaceById
			root.workspaceIds = root.workspaces.map(ws => ws.id)
		}

		function onHyprLayersChanged() {
			root.layers = DaemonSocket.hyprLayers
		}

		function onHyprActiveWorkspaceChanged() {
			root.activeWorkspace = DaemonSocket.hyprActiveWorkspace
		}
	}

	// Convenient stuff

	function toplevelsForWorkspace(workspace) {
		return ToplevelManager.toplevels.values.filter(toplevel => {
			const address = `0x${toplevel.HyprlandToplevel?.address}`
			let win = HyprlandData.windowByAddress[address]
			return win?.workspace?.id === workspace
		})
	}

	function hyprlandClientsForWorkspace(workspace) {
		return root.windowList.filter(win => win.workspace.id === workspace)
	}

	function clientForToplevel(toplevel) {
		if (!toplevel || !toplevel.HyprlandToplevel) {
			return null
		}
		const address = `0x${toplevel?.HyprlandToplevel?.address}`
		return root.windowByAddress[address]
	}

	function biggestWindowForWorkspace(workspaceId) {
		const windowsInThisWorkspace = HyprlandData.windowList.filter(w => w.workspace.id == workspaceId)
		return windowsInThisWorkspace.reduce((maxWin, win) => {
			const maxArea = (maxWin?.size?.[0] ?? 0) * (maxWin?.size?.[1] ?? 0)
			const winArea = (win?.size?.[0] ?? 0) * (win?.size?.[1] ?? 0)
			return winArea > maxArea ? win : maxWin
		}, null)
	}

	Component.onCompleted: {
		if (DaemonSocket.hyprWindows.length > 0) {
			onHyprWindowsChanged()
		}
		if (DaemonSocket.hyprMonitors.length > 0) {
			onHyprMonitorsChanged()
		}
		if (DaemonSocket.hyprWorkspaces.length > 0) {
			onHyprWorkspacesChanged()
		}
		if (Object.keys(DaemonSocket.hyprLayers).length > 0) {
			onHyprLayersChanged()
		}
		if (DaemonSocket.hyprActiveWorkspace !== null) {
			onHyprActiveWorkspaceChanged()
		}
	}
}