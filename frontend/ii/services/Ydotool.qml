pragma Singleton

import qs.services
import qs.modules.common
import Quickshell

Singleton {
    id: root
    property int shiftMode: 0 // 0: off, 1: on, 2: lock
    property list<int> shiftKeys: [42, 54]
    property list<int> altKeys: [56, 100]
    property list<int> ctrlKeys: [29, 97]
    property list<int> superKeys: [125, 126]
    property var activeModKeys: []

    function send(cmd) {
        DaemonSocket.sendCommand(cmd)
    }

    function releaseAllKeys() {
        send("releaseall")
        root.shiftMode = 0
        root.activeModKeys = []
    }

    function releaseShiftKeys() {
        for (var i = 0; i < root.shiftKeys.length; i++) {
            send("release " + root.shiftKeys[i])
        }
        root.shiftMode = 0
    }

    function releaseModKeys() {
        for (var i = 0; i < root.ctrlKeys.length; i++) {
            send("release " + root.ctrlKeys[i])
        }
        for (var i = 0; i < root.altKeys.length; i++) {
            send("release " + root.altKeys[i])
        }
        for (var i = 0; i < root.superKeys.length; i++) {
            send("release " + root.superKeys[i])
        }
        root.activeModKeys = []
    }

    function activateModKey(keycode) {
        if (root.activeModKeys.indexOf(keycode) === -1) {
            root.activeModKeys = [...root.activeModKeys, keycode]
        }
    }

    function deactivateModKey(keycode) {
        var idx = root.activeModKeys.indexOf(keycode)
        if (idx !== -1) {
            var arr = root.activeModKeys.slice()
            arr.splice(idx, 1)
            root.activeModKeys = arr
        }
    }

    function press(keycode) {
        send("press " + keycode)
    }

    function release(keycode) {
        send("release " + keycode)
    }
}
