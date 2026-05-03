pragma Singleton
pragma ComponentBehavior: Bound
import QtQuick
import Quickshell

Singleton {
    id: root

    // Mode: "auto", "tablet", "desktop"
    property string mode: "auto"

    property bool tabletMode: false
    property bool textInputActive: false
    property bool effectiveTabletMode: {
        if (mode === "tablet") return true
        if (mode === "desktop") return false
        return tabletMode
    }

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
}
