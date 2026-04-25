pragma Singleton
import QtQuick
import Quickshell
import Quickshell.Io

Singleton {
    id: root

    property bool tabletMode: false
    property bool textInputActive: false
    property bool manualOverride: false
    property bool effectiveTabletMode: manualOverride ? true : tabletMode
    property bool watcherRunning: false

    function toggleManualOverride() {
        root.manualOverride = !root.manualOverride
    }

    Process {
        id: oskWatcher
        command: [(Quickshell.env("XDG_BIN_HOME") || (Quickshell.env("HOME") + "/.local/bin")) + "/osk-watcher"]
        running: false
        stdout: SplitLineParser {
            onRead: data => {
                try {
                    const parsed = JSON.parse(data)
                    if (parsed.event === "tablet_mode") {
                        root.tabletMode = parsed.active === true
                    } else if (parsed.event === "text_focus") {
                        root.textInputActive = parsed.active === true
                    }
                } catch (e) {
                    console.warn("TabletMode: Failed to parse JSON:", e, data)
                }
            }
        }
        onRunningChanged: {
            root.watcherRunning = oskWatcher.running
            if (!oskWatcher.running) {
                Qt.callLater(() => { oskWatcher.running = true })
            }
        }
    }

    Timer {
        id: startupDelay
        interval: 2000
        repeat: false
        onTriggered: {
            oskWatcher.running = true
        }
    }

    Component.onCompleted: {
        startupDelay.start()
    }
}