pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Wayland
import Quickshell.Hyprland
import qs.services

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

	// Convenient stuff

	function toplevelsForWorkspace(workspace) {
		return ToplevelManager.toplevels.values.filter(toplevel => {
			const address = `0x${toplevel.HyprlandToplevel?.address}`;
			var win = HyprlandData.windowByAddress[address];
			return win?.workspace?.id === workspace;
		})
	}

	function hyprlandClientsForWorkspace(workspace) {
		return root.windowList.filter(win => win.workspace.id === workspace);
	}

	function clientForToplevel(toplevel) {
		if (!toplevel || !toplevel.HyprlandToplevel) {
			return null;
		}
		const address = `0x${toplevel?.HyprlandToplevel?.address}`;
		return root.windowByAddress[address];
	}

	function biggestWindowForWorkspace(workspaceId) {
		const windowsInThisWorkspace = HyprlandData.windowList.filter(w => w.workspace.id == workspaceId);
		return windowsInThisWorkspace.reduce((maxWin, win) => {
			const maxArea = (maxWin?.size?.[0] ?? 0) * (maxWin?.size?.[1] ?? 0);
			const winArea = (win?.size?.[0] ?? 0) * (win?.size?.[1] ?? 0);
			return winArea > maxArea ? win : maxWin;
		}, null);
	}

	Component.onCompleted: {}

	Connections {
		target: DaemonSocket
		onHyprlandDataUpdated: function(data) {
			if (data.windows) {
				root.windowList = data.windows
				let tempWinByAddress = {};
				for (var i = 0; i < root.windowList.length; ++i) {
					var win = root.windowList[i];
					tempWinByAddress[win.address] = win;
				}
				root.windowByAddress = tempWinByAddress;
				root.addresses = root.windowList.map(win => win.address);
			}
			if (data.monitors) {
				root.monitors = data.monitors;
			}
			if (data.workspaces) {
				// Filter out invalid workspace ids (e.g. lock-screen temp workspace 2147483647 - N)
				var rawWorkspaces = data.workspaces;
				root.workspaces = rawWorkspaces.filter(ws => ws.id >= 1 && ws.id <= 100);
				let tempWorkspaceById = {};
				for (var i = 0; i < root.workspaces.length; ++i) {
					var ws = root.workspaces[i];
					tempWorkspaceById[ws.id] = ws;
				}
				root.workspaceById = tempWorkspaceById;
				root.workspaceIds = root.workspaces.map(ws => ws.id);
			}
			if (data.layers) {
				root.layers = data.layers;
			}
			if (data.activeWorkspace) {
				root.activeWorkspace = data.activeWorkspace;
			}
		}
	}
}
