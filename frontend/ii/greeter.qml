//@ pragma UseQApplication
//@ pragma Env QS_NO_RELOAD_POPUP=1
//@ pragma Env QT_QUICK_CONTROLS_STYLE=Basic
//@ pragma Env QT_QUICK_FLICKABLE_WHEEL_DECELERATION=10000

// Remove two slashes below and change the value to change the UI scale
////@ pragma Env QT_SCALE_FACTOR=1

// Greeter shell — minimal Quickshell config for the display manager lock screen.
// Loads only: background, lock screen, theme, and essential services.
// On auth success, the daemon swaps to the full shell config (shell.qml).

import "modules/common"
import "services"

import QtQuick
import Quickshell
import Quickshell.Io
import Quickshell.Hyprland

ShellRoot {
    id: root

    property bool lockTriggered: false

    Component.onCompleted: {
        MaterialThemeLoader.reapplyTheme()
        Wallpapers.load()
    }

    // Auto-lock when services are ready.
    // In greeter mode, the lock screen IS the UI.
    Connections {
        target: Config
        function onReadyChanged() {
            root.triggerLock()
        }
    }
    Connections {
        target: Persistent
        function onReadyChanged() {
            root.triggerLock()
        }
    }

    function triggerLock() {
        if (lockTriggered) return
        if (!Config.ready || !Persistent.ready) return
        lockTriggered = true
        DaemonSocket.lockStartup()
    }

    // Background only — no bar, dock, or other panels.
    Loader {
        active: true
        source: "modules/ii/background/Background.qml"
    }

    // Lock screen — the primary UI of the greeter.
    Loader {
        active: true
        source: "modules/ii/lock/Lock.qml"
    }
}
