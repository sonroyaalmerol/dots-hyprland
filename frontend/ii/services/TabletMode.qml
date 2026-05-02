pragma Singleton
import QtQuick
import Quickshell
import Quickshell.Io

Singleton {
    id: root

    // Mode: "auto", "tablet", "desktop"
    property string mode: "auto"

    property bool tabletMode: false      // from hardware sensor
    property bool textInputActive: false  // from snry-daemon
    property bool effectiveTabletMode: {
        if (mode === "tablet") return true
        if (mode === "desktop") return false
        return tabletMode // auto mode
    }
    property bool watcherRunning: daemonProc.running

    // Wrapper exposing .connected and .write() for Ydotool compatibility
    property alias daemonSocket: daemonBridge
    QtObject {
        id: daemonBridge
        property bool connected: daemonProc.running
        function write(data) {
            daemonProc.write(data)
        }
    }

    function cycleMode() {
        if (root.mode === "auto") root.mode = "tablet"
        else if (root.mode === "tablet") root.mode = "desktop"
        else root.mode = "auto"
    }

    function setMode(newMode) {
        if (newMode === "auto" || newMode === "tablet" || newMode === "desktop") {
            root.mode = newMode
        }
    }

    // Python relay process: maintains a persistent connection to snry-daemon
    // with automatic reconnection. Handles bidirectional communication:
    //   daemon socket → stdout (JSON events for QML to parse)
    //   stdin → daemon socket (commands from Ydotool)
    Process {
        id: daemonProc
        running: true

        command: ["python3", "-u", "-c", `
import socket, os, sys, select, time

sock_path = os.path.join(os.environ.get('XDG_RUNTIME_DIR', ''), 'snry-daemon.sock')

def relay(s):
    print('CONNECTED', flush=True)
    while True:
        rlist, _, _ = select.select([s, sys.stdin], [], [], 30)
        if s in rlist:
            data = s.recv(65536)
            if not data:
                return
            sys.stdout.buffer.write(data)
            sys.stdout.buffer.flush()
        if sys.stdin in rlist:
            line = sys.stdin.buffer.readline()
            if not line:
                return
            nl = bytes([10])
            s.sendall(line.rstrip(nl) + nl)

while True:
    try:
        if os.path.exists(sock_path):
            s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            s.settimeout(2)
            s.connect(sock_path)
            s.settimeout(None)
            relay(s)
            s.close()
    except Exception:
        pass
    time.sleep(2)
`]

        stdout: SplitParser {
            onRead: data => {
                if (data === "CONNECTED") {
                    console.log("TabletMode: connected to snry-daemon")
                    return
                }
                try {
                    const parsed = JSON.parse(data)
                    if (parsed.event === "tablet_mode") {
                        root.tabletMode = parsed.active === true
                    } else if (parsed.event === "text_focus") {
                        root.textInputActive = parsed.active === true
                    }
                } catch (e) {
                    console.warn("TabletMode: parse error:", e, data)
                }
            }
        }
    }
}
