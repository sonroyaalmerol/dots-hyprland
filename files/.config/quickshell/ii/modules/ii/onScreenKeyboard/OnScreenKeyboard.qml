import qs
import qs.services
import qs.modules.common
import qs.modules.common.widgets
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import Quickshell.Io
import Quickshell
import Quickshell.Wayland
import Quickshell.Hyprland

Scope { // Scope
    id: root
    property bool pinned: Config.options?.osk.pinnedOnStartup ?? false

    // Auto-show/hide state tracking
    property bool autoShown: false
    property bool manualDismissed: false
    property bool overviewForcedOsk: false

    Timer {
        id: autoShowTimer
        interval: 300
        onTriggered: {
            if (TabletMode.effectiveTabletMode && TabletMode.textInputActive && !root.manualDismissed && !GlobalStates.oskOpen) {
                GlobalStates.oskOpen = true
                root.autoShown = true
            }
        }
    }

    Connections {
        target: TabletMode

        function onTextInputActiveChanged() {
            if (TabletMode.textInputActive) {
                // New text focus — reset manual dismiss
                root.manualDismissed = false
                // Auto-show if in tablet mode
                if (TabletMode.effectiveTabletMode && !GlobalStates.oskOpen) {
                    autoShowTimer.start()
                }
            } else {
                // Text focus lost — auto-hide if we auto-showed
                autoShowTimer.stop()
                if (root.autoShown) {
                    GlobalStates.oskOpen = false
                    root.autoShown = false
                    root.manualDismissed = false
                }
            }
        }

        function onEffectiveTabletModeChanged() {
            if (!TabletMode.effectiveTabletMode) {
                if (root.autoShown || root.overviewForcedOsk) {
                    // Left tablet mode — auto-hide
                    autoShowTimer.stop()
                    GlobalStates.oskOpen = false
                    root.autoShown = false
                    root.overviewForcedOsk = false
                }
            } else if (TabletMode.textInputActive && !root.manualDismissed && !GlobalStates.oskOpen) {
                // Entered tablet mode with active text focus — auto-show
                autoShowTimer.start()
            }
        }
    }

    Connections {
        target: GlobalStates

        function onOverviewOpenChanged() {
            if (GlobalStates.overviewOpen && TabletMode.effectiveTabletMode) {
                // Overview opened in tablet mode — force OSK open
                GlobalStates.oskOpen = true
                root.overviewForcedOsk = true
                root.autoShown = false
            } else if (!GlobalStates.overviewOpen && root.overviewForcedOsk) {
                // Overview closed — hide OSK if it was only open for overview
                GlobalStates.oskOpen = false
                root.overviewForcedOsk = false
            }
        }
    }

    component OskControlButton: GroupButton { // Pin button
        baseWidth: 40
        baseHeight: 40
        clickedWidth: baseWidth
        clickedHeight: baseHeight + 10
        buttonRadius: Appearance.rounding.normal
    }

    Loader {
        id: oskLoader
        active: GlobalStates.oskOpen
        onActiveChanged: {
            if (!oskLoader.active) {
                Ydotool.releaseAllKeys();
            }
        }
        
        sourceComponent: PanelWindow { // Window
            id: oskRoot
            visible: oskLoader.active && !GlobalStates.screenLocked
            property real floatOffsetY: 0

            anchors {
                bottom: true
                left: true
                right: true
            }
            margins {
                bottom: root.pinned ? 0 : floatOffsetY
            }

            onFloatOffsetYChanged: {
                if (root.pinned) floatOffsetY = 0
            }

            function hide() {
                GlobalStates.oskOpen = false
                root.autoShown = false
                root.manualDismissed = true
            }
            exclusiveZone: root.pinned ? implicitHeight - Appearance.sizes.hyprlandGapsOut : 0
            implicitWidth: oskBackground.width + Appearance.sizes.elevationMargin * 2
            implicitHeight: oskBackground.height + Appearance.sizes.elevationMargin * 2
            WlrLayershell.namespace: "quickshell:osk"
            WlrLayershell.layer: WlrLayer.Overlay
            // Hyprland 0.49: Focus is always exclusive and setting this breaks mouse focus grab
            // WlrLayershell.keyboardFocus: WlrKeyboardFocus.Exclusive
            color: "transparent"

            mask: Region {
                item: oskBackground
            }

            // Make it usable with other panels
            Component.onCompleted: {
                GlobalFocusGrab.addPersistent(oskRoot);
            }
            Component.onDestruction: {
                GlobalFocusGrab.removePersistent(oskRoot);
            }

            // Background
            StyledRectangularShadow {
                target: oskBackground
            }
            Rectangle {
                id: oskBackground
                anchors.centerIn: parent
                color: Appearance.colors.colLayer0
                radius: Appearance.rounding.windowRounding
                property real padding: 10
                implicitWidth: oskRowLayout.implicitWidth + padding * 2
                implicitHeight: oskRowLayout.implicitHeight + padding * 2

                Keys.onPressed: (event) => { // Esc to close
                    if (event.key === Qt.Key_Escape) {
                        oskRoot.hide()
                    }
                }

                RowLayout {
                    id: oskRowLayout
                    anchors.centerIn: parent
                    spacing: 5
                    ColumnLayout {
                        id: controlsColumn
                        Layout.fillHeight: true
                        spacing: 0

                        // Drag handle (only when unpinned)
                        MouseArea {
                            id: dragHandle
                            Layout.fillWidth: true
                            Layout.preferredHeight: 24
                            Layout.topMargin: 5
                            enabled: !root.pinned
                            visible: !root.pinned
                            acceptedButtons: Qt.LeftButton
                            hoverEnabled: true
                            cursorShape: pressed ? Qt.ClosedHandCursor : Qt.OpenHandCursor

                            property real _startY: 0
                            property real _startOffset: 0

                            onPressed: (mouse) => {
                                _startY = mouse.y
                                _startOffset = oskRoot.floatOffsetY
                            }
                            onDoubleClicked: oskRoot.floatOffsetY = 0
                            onPositionChanged: (mouse) => {
                                if (pressed) {
                                    // finger moves up (mouse.y decreases) = window should move up
                                    // margins.bottom increases = window moves up
                                    var delta = _startY - mouse.y
                                    oskRoot.floatOffsetY = Math.max(0, _startOffset + delta)
                                }
                            }

                            Rectangle {
                                anchors.centerIn: parent
                                width: 20
                                height: 4
                                radius: 2
                                color: Appearance.colors.colOutlineVariant
                                opacity: dragHandle.containsMouse ? 1 : 0.5
                            }
                        }

                        VerticalButtonGroup {
                            OskControlButton { // Pin button
                                toggled: root.pinned
                                downAction: () => root.pinned = !root.pinned
                                contentItem: MaterialSymbol {
                                    text: "keep"
                                    horizontalAlignment: Text.AlignHCenter
                                    iconSize: Appearance.font.pixelSize.larger
                                    color: root.pinned ? Appearance.m3colors.m3onPrimary : Appearance.colors.colOnLayer0
                                }
                            }
                            OskControlButton {
                                onClicked: () => {
                                    oskRoot.hide()
                                }
                                contentItem: MaterialSymbol {
                                    horizontalAlignment: Text.AlignHCenter
                                    text: "keyboard_hide"
                                    iconSize: Appearance.font.pixelSize.larger
                                }
                            }
                        }
                    }
                    Rectangle {
                        Layout.topMargin: 20
                        Layout.bottomMargin: 20
                        Layout.fillHeight: true
                        implicitWidth: 1
                        color: Appearance.colors.colOutlineVariant
                    }
                    OskContent {
                        id: oskContent
                        Layout.fillWidth: true
                    }
                }
            }

        }
    }

    IpcHandler {
        target: "osk"

        function toggle(): void {
            if (GlobalStates.oskOpen) {
                GlobalStates.oskOpen = false
                root.autoShown = false
                root.manualDismissed = true
            } else {
                GlobalStates.oskOpen = true
                root.autoShown = false
            }
        }

        function close(): void {
            GlobalStates.oskOpen = false
            root.autoShown = false
            root.manualDismissed = true
        }

        function open(): void {
            GlobalStates.oskOpen = true
            root.autoShown = false
        }
    }

    GlobalShortcut {
        name: "oskToggle"
        description: "Toggles on screen keyboard on press"

        onPressed: {
            if (GlobalStates.oskOpen) {
                GlobalStates.oskOpen = false
                root.autoShown = false
                root.manualDismissed = true
            } else {
                GlobalStates.oskOpen = true
                root.autoShown = false
            }
        }
    }

    GlobalShortcut {
        name: "oskOpen"
        description: "Opens on screen keyboard on press"

        onPressed: {
            GlobalStates.oskOpen = true
            root.autoShown = false
        }
    }

    GlobalShortcut {
        name: "oskClose"
        description: "Closes on screen keyboard on press"

        onPressed: {
            GlobalStates.oskOpen = false
            root.autoShown = false
            root.manualDismissed = true
        }
    }

}
