pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Hyprland

import qs.services
import qs.modules.common
import qs.modules.common.functions

/**
 * Configs Hyprland
 */
Singleton {
    id: root
    
    signal reloaded()

    readonly property string shellOverridesPath: FileUtils.trimFileProtocol(`${Directories.config}/hypr/hyprland/shellOverrides/main.conf`)

    function set(key: string, value: var) {
        DaemonSocket.sendCommand(`hyprconfig-edit --file ${root.shellOverridesPath} --set ${key} ${value}`)
    }

    function setMany(entries: var) {
        let cmd = `hyprconfig-edit --file ${root.shellOverridesPath}`
        for (let key in entries) {
            cmd += ` --set ${key} ${entries[key]}`
        }
        DaemonSocket.sendCommand(cmd)
    }

    function reset(key: string) {
        DaemonSocket.sendCommand(`hyprconfig-edit --file ${root.shellOverridesPath} --reset ${key}`)
    }

    function resetMany(keys: list<string>) {
        let cmd = `hyprconfig-edit --file ${root.shellOverridesPath}`
        for (let i = 0; i < keys.length; i++) {
            cmd += ` --reset ${keys[i]}`
        }
        DaemonSocket.sendCommand(cmd)
    }

    Connections {
        target: Hyprland

        function onRawEvent(event) {
            if (event.name == "configreloaded") {
                root.reloaded()
            }
        }
    }
}
