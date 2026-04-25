pragma Singleton

import qs.modules.common
import Quickshell
import Quickshell.Io

Singleton {
    id: root
    property int shiftMode: 0 // 0: off, 1: on, 2: lock
    property list<int> shiftKeys: [42, 54]
    property list<int> altKeys: [56, 100]
    property list<int> ctrlKeys: [29, 97]

    // Persistent osk-input process
    Process {
        id: oskInput
        running: true
        stdinEnabled: true
        command: [(Quickshell.env("XDG_BIN_HOME") || (Quickshell.env("HOME") + "/.local/bin")) + "/osk-input"]
        stdout: SplitParser {
            onRead: data => {
                // osk-input logs to stderr
            }
        }
        onExited: (exitCode, exitStatus) => {
            console.warn("Ydotool: osk-input exited, restarting...")
            Qt.callLater(() => { oskInput.running = true })
        }
    }

    function send(cmd) {
        if (oskInput.running) {
            oskInput.write(cmd + "\n")
        }
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
