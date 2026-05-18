pragma Singleton
pragma ComponentBehavior: Bound

import qs.services
import QtQuick
import Quickshell
import Quickshell.Hyprland

/**
 * Provides access to Hyprland data via Quickshell.Hyprland instead of daemon relay.
 * Monitors, workspaces, and toplevels come directly from the Hyprland IPC module.
 * Extended window data (size, pid, class, etc.) comes from HyprlandToplevel.lastIpcObject.
 * Layers are tracked from Hyprland raw events.
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

	// ── Monitors ─────────────────────────────────────────────────────────────

	function updateMonitors() {
		var result = []
		for (var i = 0; i < Hyprland.monitors.count; ++i) {
			var mon = Hyprland.monitors.at(i)
			if (!mon) continue
			var obj = {
				"id": mon.id,
				"name": mon.name,
				"description": mon.description,
				"width": mon.width,
				"height": mon.height,
				"scale": mon.scale,
				"x": mon.x,
				"y": mon.y,
				"focused": mon.focused,
			}
			if (mon.activeWorkspace) {
				obj["activeWorkspace"] = {
					"id": mon.activeWorkspace.id,
					"name": mon.activeWorkspace.name,
				}
			}
			// Merge in lastIpcObject fields (refreshRate, etc.)
			var ipcObj = mon.lastIpcObject
			if (ipcObj) {
				for (var k in ipcObj) {
					if (!(k in obj))
						obj[k] = ipcObj[k]
				}
			}
			result.push(obj)
		}
		root.monitors = result
	}

	// ── Workspaces ───────────────────────────────────────────────────────────

	function updateWorkspaces() {
		var result = []
		for (var i = 0; i < Hyprland.workspaces.count; ++i) {
			var ws = Hyprland.workspaces.at(i)
			if (!ws) continue
			result.push({
				"id": ws.id,
				"name": ws.name,
				"monitorName": ws.monitor ? ws.monitor.name : "",
			})
		}
		root.workspaces = result.filter(ws => ws.id >= 1 && ws.id <= 100)
		var byId = {}
		for (var i = 0; i < root.workspaces.length; ++i) {
			byId[root.workspaces[i].id] = root.workspaces[i]
		}
		root.workspaceById = byId
		root.workspaceIds = root.workspaces.map(ws => ws.id)
	}

	// ── Active workspace ─────────────────────────────────────────────────────

	function updateActiveWorkspace() {
		var fw = Hyprland.focusedWorkspace
		if (!fw) {
			root.activeWorkspace = null
			return
		}
		var obj = {
			"id": fw.id,
			"name": fw.name,
		}
		var fm = Hyprland.focusedMonitor
		if (fm) {
			obj["monitor"] = fm.name
		}
		var at = Hyprland.activeToplevel
		if (at) {
			obj["activeWindow"] = "0x" + at.address
		}
		root.activeWorkspace = obj
	}

	// ── Windows / Toplevels ──────────────────────────────────────────────────

	function updateWindows() {
		var result = []
		var byAddr = {}
		var addrList = []
		for (var i = 0; i < Hyprland.toplevels.count; ++i) {
			var tl = Hyprland.toplevels.at(i)
			if (!tl) continue
			var addr = "0x" + tl.address
			var obj = {
				"address": addr,
				"title": tl.title,
			}
			if (tl.workspace) {
				obj["workspace"] = {
					"id": tl.workspace.id,
					"name": tl.workspace.name,
				}
			}
			if (tl.monitor) {
				obj["monitor"] = tl.monitor.id
			}
			// Merge lastIpcObject for extended fields (class, size, pid, floating, etc.)
			var ipcObj = tl.lastIpcObject
			if (ipcObj) {
				for (var k in ipcObj) {
					if (!(k in obj))
						obj[k] = ipcObj[k]
				}
			}
			result.push(obj)
			byAddr[addr] = obj
			addrList.push(addr)
		}
		root.windowList = result
		root.windowByAddress = byAddr
		root.addresses = addrList
	}

	// ── Layers ───────────────────────────────────────────────────────────────

	function updateLayers() {
		// Layers are not provided by Quickshell.Hyprland directly.
		// Tracked from raw events below.
	}

	// ── Connections to Quickshell.Hyprland ───────────────────────────────────

	Connections {
		target: Hyprland

		function onFocusedMonitorChanged() {
			root.updateMonitors()
			root.updateActiveWorkspace()
		}
		function onFocusedWorkspaceChanged() {
			root.updateWorkspaces()
			root.updateActiveWorkspace()
		}
		function onActiveToplevelChanged() {
			root.updateWindows()
			root.updateActiveWorkspace()
		}

		function onRawEvent(event) {
			var name = event.name
			var data = event.data

			// Layer events
			if (name === "openlayer") {
				var l = Object.assign({}, root.layers)
				l[data] = true
				root.layers = l
			} else if (name === "closelayer") {
				var l = Object.assign({}, root.layers)
				delete l[data]
				root.layers = l
			}

			// Workspace events — refresh workspace/monitor data
			if (name === "workspacev2" || name === "createworkspacev2" || name === "destroyworkspacev2"
				|| name === "moveworkspacev2" || name === "renameworkspace"
				|| name === "focusedmonv2" || name === "focusedmon"
				|| name === "activespecialv2") {
				root.updateWorkspaces()
				root.updateMonitors()
				root.updateActiveWorkspace()
			}

			// Window events — refresh window data
			if (name === "openwindow" || name === "closewindow" || name === "kill"
				|| name === "movewindowv2" || name === "movewindow"
				|| name === "windowtitlev2" || name === "windowtitle"
				|| name === "changefloatingmode" || name === "fullscreen"
				|| name === "pin" || name === "minimized" || name === "urgent"
				|| name === "activewindowv2") {
				// HyprlandToplevel model updates automatically,
				// but lastIpcObject may lag. Schedule a delayed refresh.
				refreshWindowsTimer.restart()
			}

			// Monitor events
			if (name === "monitoraddedv2" || name === "monitorremovedv2") {
				root.updateMonitors()
			}
		}
	}

	// Window data may need a delayed refresh to pick up lastIpeObject updates
	// after toplevel model has processed the event.
	Timer {
		id: refreshWindowsTimer
		interval: 100
		onTriggered: root.updateWindows()
	}

	// ── Convenient stuff ─────────────────────────────────────────────────────

	function toplevelsForWorkspace(workspace) {
		return ToplevelManager.toplevels.values.filter(toplevel => {
			const address = `0x${toplevel.HyprlandToplevel?.address}`
			let win = HyprlandData.windowByAddress[address]
			return win?.workspace?.id === workspace
		})
	}

	function hyprlandClientsForWorkspace(workspace) {
		return root.windowList.filter(win => win.workspace?.id === workspace)
	}

	function clientForToplevel(toplevel) {
		if (!toplevel || !toplevel.HyprlandToplevel) {
			return null
		}
		const address = `0x${toplevel?.HyprlandToplevel?.address}`
		return root.windowByAddress[address]
	}

	function biggestWindowForWorkspace(workspaceId) {
		const windowsInThisWorkspace = HyprlandData.windowList.filter(w => w.workspace?.id == workspaceId)
		return windowsInThisWorkspace.reduce((maxWin, win) => {
			const maxArea = (maxWin?.size?.[0] ?? 0) * (maxWin?.size?.[1] ?? 0)
			const winArea = (win?.size?.[0] ?? 0) * (win?.size?.[1] ?? 0)
			return winArea > maxArea ? win : maxWin
		}, null)
	}

	Component.onCompleted: {
		root.updateMonitors()
		root.updateWorkspaces()
		root.updateWindows()
		root.updateActiveWorkspace()
	}
}
