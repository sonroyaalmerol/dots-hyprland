pragma ComponentBehavior: Bound
import QtQml
import QtQuick
import qs.services
import "../"

NestableObject {
    id: root

    required property string key
    property bool fetching: false
    property bool set
    property var value

    Component.onCompleted: fetch()

    Connections {
        target: HyprlandConfig
        function onReloaded() {
            root.fetch();
        }
    }

    function fetch() {
        root.fetching = true
        DaemonSocket.hyprconfigGet(root.key)
    }

    function setValue(newValue) {
        HyprlandConfig.set(root.key, newValue)
    }

    function reset() {
        HyprlandConfig.reset(root.key)
    }

    Connections {
        target: DaemonSocket
        function onHyprconfigValue(key, value) {
            if (key !== root.key) return
            root.fetching = false
            if (value == "no such option")
                return;
            try {
                const obj = JSON.parse(value);
                for (const k in obj) {
                    if (k == "option")
                        continue;
                    else if (k == "set")
                        root.set = obj[k];
                    else
                        root.value = obj[k];
                }
            } catch (e) {
                console.log(`[HyprlandConfigOption] Failed to fetch option "${root.key}":\n  - Output: ${value.trim()}\n  - Error: ${e}`);
            }
        }
    }
}