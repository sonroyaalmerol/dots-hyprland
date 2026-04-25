import QtQuick
import Quickshell
import qs
import qs.services
import qs.modules.common
import qs.modules.common.functions
import qs.modules.common.widgets

QuickToggleModel {
    name: Translation.tr("Dark Mode")
    statusText: Appearance.m3colors.darkmode ? Translation.tr("Dark") : Translation.tr("Light")

    toggled: Appearance.m3colors.darkmode
    icon: "contrast"
    
    mainAction: () => {
        Quickshell.execDetached(["gsettings", "set", "org.gnome.desktop.interface", "color-scheme", Appearance.m3colors.darkmode ? "prefer-light" : "prefer-dark"]);
    }

    tooltipText: Translation.tr("Dark Mode")
}
