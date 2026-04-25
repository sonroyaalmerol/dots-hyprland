pragma Singleton
import QtQuick
import Quickshell
import Quickshell.Io

Singleton {
    id: root

    // Mode: "auto", "tablet", "desktop"
    property string mode: "auto"

    property bool tabletMode: false      // from hardware sensor
    property bool textInputActive: false  // from snry-daemon
    property bool effectiveTabletMode: {
        if (mode === "tablet") return true
        if (mode === "desktop") return false
        return tabletMode // auto mode
    }
    property bool watcherRunning: false

    // Expose the daemon process so Ydotool can write to stdin
    property alias daemonProcess: oskWatcher

    function cycleMode() {
        if (root.mode === "auto") root.mode = "tablet"
        else if (root.mode === "tablet") root.mode = "desktop"
        else root.mode = "auto"
    }

    function setMode(newMode) {
        if (newMode === "auto" || newMode === "tablet" || newMode === "desktop") {
            root.mode = newMode
        }
    }

    // Single snry-daemon process: tablet mode + input method + idle + uinput
    Process {
        id: oskWatcher
        stdinEnabled: true
        command: [(Quickshell.env("XDG_BIN_HOME") || (Quickshell.env("HOME") + "/.local/bin")) + "/snry-daemon"]
        running: false
        stdout: SplitParser {
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
