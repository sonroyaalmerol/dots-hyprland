import QtQuick
import Quickshell
import qs
import qs.services
import qs.modules.common
import qs.modules.common.functions
import qs.modules.common.widgets

QuickToggleModel {
    name: {
        if (TabletMode.mode === "tablet") return Translation.tr("Tablet Mode")
        if (TabletMode.mode === "desktop") return Translation.tr("Desktop Mode")
        return Translation.tr("Auto Mode")
    }
    toggled: TabletMode.effectiveTabletMode
    icon: {
        if (TabletMode.mode === "tablet") return "tablet"
        if (TabletMode.mode === "desktop") return "monitor"
        return "devices"  // auto mode icon
    }
    mainAction: () => {
        TabletMode.cycleMode()
    }
    tooltipText: {
        if (TabletMode.mode === "tablet") return Translation.tr("Forced tablet mode")
        if (TabletMode.mode === "desktop") return Translation.tr("Forced desktop mode")
        return Translation.tr("Auto-detect mode")
    }
}