pragma Singleton

import qs.modules.common
import qs.modules.common.functions
import qs.services
import QtQuick
import Quickshell

/*
 * System updates service. Currently only supports Arch.
 */
Singleton {
    id: root

    property bool available: false
    property bool checking: false
    property int count: 0
    
    readonly property bool updateAdvised: available && count > Config.options.updates.adviseUpdateThreshold
    readonly property bool updateStronglyAdvised: available && count > Config.options.updates.stronglyAdviseUpdateThreshold

    Connections {
        target: DaemonSocket
        function onUpdatesDataUpdated(data) {
            root.available = data.available ?? false;
            root.count = data.count ?? 0;
        }
    }

    function load() {}
    function refresh() {
        // Daemon handles periodic polling
    }
}
