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

    // Expose the daemon socket so Ydotool can write commands
    property alias daemonSocket: sock

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

    // Connect to snry-daemon via Unix socket
    Socket {
        id: sock
        path: Quickshell.env("XDG_RUNTIME_DIR") + "/snry-daemon.sock"
        connected: true

        parser: SplitParser {
            onRead: data => {
                try {
                    const parsed = JSON.parse(data)
                    if (parsed.event === "tablet_mode") {
                        root.tabletMode = parsed.active === true
                    } else if (parsed.event === "text_focus") {
                        root.textInputActive = parsed.active === true
                    }
                } catch (e) {
                    console.warn("TabletMode: parse error:", e, data)
                }
            }
        }

        onSocketConnected: {
            root.watcherRunning = true
        }

        onConnectionStateChanged: {
            root.watcherRunning = sock.connected
            if (!sock.connected) {
                // Reconnect after delay
                reconnectTimer.start()
            }
        }

        onSocketError: error => {
            console.warn("TabletMode socket error:", error)
            reconnectTimer.start()
        }
    }

    Timer {
        id: reconnectTimer
        interval: 2000
        repeat: false
        onTriggered: {
            sock.connected = true
        }
    }
}
