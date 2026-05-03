pragma Singleton
pragma ComponentBehavior: Bound

import qs.services
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

	function updateWindows() {
		root.windowList = DaemonSocket.hyprWindows
		let tempWinByAddress = {}
		for (let i = 0; i < root.windowList.length; ++i) {
			let win = root.windowList[i]
			tempWinByAddress[win.address] = win
		}
		root.windowByAddress = tempWinByAddress
		root.addresses = root.windowList.map(win => win.address)
	}

	function updateMonitors() {
		root.monitors = DaemonSocket.hyprMonitors
	}

	function updateWorkspaces() {
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

	function updateLayers() {
		root.layers = DaemonSocket.hyprLayers
	}

	function updateActiveWorkspace() {
		root.activeWorkspace = DaemonSocket.hyprActiveWorkspace
	}

	Connections {
		target: DaemonSocket

		onHyprWindowsChanged: root.updateWindows()
		onHyprMonitorsChanged: root.updateMonitors()
		onHyprWorkspacesChanged: root.updateWorkspaces()
		onHyprLayersChanged: root.updateLayers()
		onHyprActiveWorkspaceChanged: root.updateActiveWorkspace()
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
		if (DaemonSocket.hyprWindows.length > 0) root.updateWindows()
		if (DaemonSocket.hyprMonitors.length > 0) root.updateMonitors()
		if (DaemonSocket.hyprWorkspaces.length > 0) root.updateWorkspaces()
		if (Object.keys(DaemonSocket.hyprLayers).length > 0) root.updateLayers()
		if (DaemonSocket.hyprActiveWorkspace !== null) root.updateActiveWorkspace()
	}
}