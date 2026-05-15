//@ pragma UseQApplication
//@ pragma Env QS_NO_RELOAD_POPUP=1
//@ pragma Env QT_QUICK_CONTROLS_STYLE=Basic
//@ pragma Env QT_QUICK_FLICKABLE_WHEEL_DECELERATION=10000

// Remove two slashes below and change the value to change the UI scale
////@ pragma Env QT_SCALE_FACTOR=1

// Greeter shell — minimal Quickshell config for the display manager lock screen.
// Loaded by snry-dm (system service) on VT1 as the graphical greeter.
// On auth success, the DM kills this and starts the user's session.

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

    // Auto-lock immediately when services are ready.
    // In DM mode, the lock screen IS the entire UI.
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
        // In DM mode, send lock-startup so the lock screen activates.
        // The DM socket handles this by acknowledging (no auto-unlock).
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
