pragma Singleton
pragma ComponentBehavior: Bound

import qs.modules.common
import QtQuick
import Quickshell
import Quickshell.Io

/**
 * Automatically reloads generated material colors.
 * It is necessary to run reapplyTheme() on startup because Singletons are lazily loaded.
 */
Singleton {
	id: root
	property string filePath: Directories.generatedMaterialThemePath

	function reapplyTheme() {
		themeFileView.reload()
	}

	function applyColors(fileContent) {
		const json = JSON.parse(fileContent)
		for (const key in json) {
			if (json.hasOwnProperty(key)) {
				// Convert snake_case to CamelCase
				const camelCaseKey = key.replace(/_([a-z])/g, (g) => g[1].toUpperCase())
				const m3Key = `m3${camelCaseKey}`
				Appearance.m3colors[m3Key] = json[key]
			}
		}
	}

	function resetFilePathNextTime() {
		resetFilePathNextWallpaperChange.enabled = true
	}

	Connections {
		id: resetFilePathNextWallpaperChange
		enabled: false
		target: Config.options.background
		function onWallpaperPathChanged() {
			root.filePath = ""
			root.filePath = Directories.generatedMaterialThemePath
			resetFilePathNextWallpaperChange.enabled = false
		}
	}

	Timer {
		id: delayedFileRead
		interval: Config.options?.hacks?.arbitraryRaceConditionDelay ?? 100
		repeat: false
		running: false
		onTriggered: {
			root.applyColors(themeFileView.text())
		}
	}

	FileView { 
		id: themeFileView
		path: Qt.resolvedUrl(root.filePath)
		watchChanges: true
		onFileChanged: {
			this.reload()
			delayedFileRead.start()
		}
		onLoadedChanged: {
			const fileContent = themeFileView.text()
			root.applyColors(fileContent)
		}
		onLoadFailed: root.resetFilePathNextTime();
	}

	Component.onCompleted: {
		gsettingsInitProc.running = true
		gsettingsMonitorProc.running = true
	}

	function setDarkmodeFromGsettings(value: string) {
		Appearance.m3colors.darkmode = (value === "prefer-dark")
	}

	Process {
		id: gsettingsInitProc
		command: ["gsettings", "get", "org.gnome.desktop.interface", "color-scheme"]
		stdout: SplitParser {
			onRead: data => {
				const value = data.trim().replace(/^'|'$/g, "")
				root.setDarkmodeFromGsettings(value)
			}
		}
	}

	Process {
		id: gsettingsMonitorProc
		command: ["gsettings", "monitor", "org.gnome.desktop.interface", "color-scheme"]
		stdout: SplitParser {
			onRead: data => {
				const value = data.trim()
				root.setDarkmodeFromGsettings(value)
			}
		}
		onExited: {
			if (gsettingsMonitorProc.running) {
				gsettingsMonitorProc.running = true
			}
		}
	}
}
