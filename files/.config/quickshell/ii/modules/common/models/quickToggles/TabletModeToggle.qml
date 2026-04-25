import QtQuick
import Quickshell
import qs
import qs.services
import qs.modules.common
import qs.modules.common.functions
import qs.modules.common.widgets

QuickToggleModel {
    name: Translation.tr("Tablet Mode")

    toggled: TabletMode.effectiveTabletMode
    icon: toggled ? "tablet" : "keyboard"
    mainAction: () => {
        TabletMode.toggleManualOverride()
    }
    tooltipText: Translation.tr("Toggle tablet mode")
}