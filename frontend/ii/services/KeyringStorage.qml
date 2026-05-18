pragma Singleton
pragma ComponentBehavior: Bound

import qs
import qs.modules.common
import qs.modules.common.functions
import Quickshell;
import Quickshell.Io;
import QtQuick;

/**
 * For storing sensitive data in the keyring.
 * Use this for small data only, since it stores a JSON of the contents directly and doesn't use a database.
 */
Singleton {
    id: root

    signal dataChanged()

    property bool loaded: false
    property var keyringData: ({})
    
    property var properties: {
        "application": "snry-shell",
        "explanation": Translation.tr("For storing API keys and other sensitive information"),
    }
    property var propertiesAsArgs: Object.keys(root.properties).reduce(
        function(arr, key) {
            return arr.concat([key, root.properties[key]]);
        }, []
    )
    property string keyringLabel: Translation.tr("%1 Safe Storage").arg("snry-shell")

    function setNestedField(path, value) {
        if (!root.keyringData) root.keyringData = {};
        let keys = path;
        let obj = root.keyringData;
        let parents = [obj];

        // Traverse and collect parent objects
        for (let i = 0; i < keys.length - 1; ++i) {
            if (!obj[keys[i]] || typeof obj[keys[i]] !== "object") {
                obj[keys[i]] = {};
            }
            obj = obj[keys[i]];
            parents.push(obj);
        }

        // Set the value at the innermost key
        obj[keys[keys.length - 1]] = value;

        // Reassign each parent object from the bottom up to trigger change notifications
        for (let i = keys.length - 2; i >= 0; --i) {
            let parent = parents[i];
            let key = keys[i];
            // Shallow clone to change object identity (spread replaced with Object.assign)
            parent[key] = Object.assign({}, parent[key]);
        }

        // Finally, reassign root.keyringData to trigger top-level change
        root.keyringData = Object.assign({}, root.keyringData);

        saveKeyringData();
    }

    function fetchKeyringData() {
        DaemonSocket.sendCommand("keyring lookup");
    }

    function saveKeyringData() {
        saveData.stdinEnabled = true;
        saveData.running = true;
    }

    Connections {
        target: DaemonSocket
        function onKeyringLookupResult(data) {
            if (data.status === "not_found") {
                console.error("[KeyringStorage] Entry not found, initializing.");
                root.keyringData = {};
                saveKeyringData();
            } else if (data.status === "ok") {
                const text = data.data;
                if (text && text.length > 0 && text.startsWith("{")) {
                    try {
                        root.keyringData = JSON.parse(text);
                    } catch (e) {
                        console.error("[KeyringStorage] Failed to get keyring data, reinitializing.");
                        root.keyringData = {};
                        saveKeyringData();
                    }
                } else {
                    root.keyringData = {};
                }
            }
            root.loaded = true;
        }
    }

    Process {
        id: saveData
        command: [
            "secret-tool", "store", "--label=" + keyringLabel,
            ...propertiesAsArgs,
        ]
        onRunningChanged: {
            if (saveData.running) {
                saveData.write(JSON.stringify(root.keyringData));
                root.dataChanged()
                stdinEnabled = false
            }
        }
    }
    
}
