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

    function send(cmd) {
        DaemonSocket.sendCommand(cmd)
    }

    function releaseAllKeys() {
        send("releaseall")
        root.shiftMode = 0
    }

    function releaseShiftKeys() {
        for (var i = 0; i < root.shiftKeys.length; i++) {
            send("release " + root.shiftKeys[i])
        }
        root.shiftMode = 0
    }

    function press(keycode) {
        send("press " + keycode)
    }

    function release(keycode) {
        send("release " + keycode)
    }
}
