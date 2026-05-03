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

    // Bind directly to daemon state.
    property bool oskActive: DaemonSocket.oskVisible && !GlobalStates.screenLocked

    Connections {
        target: GlobalStates

        function onOverviewOpenChanged() {
            if (GlobalStates.overviewOpen && DaemonSocket.effectiveTabletMode) {
                DaemonSocket.oskShow()
            } else if (!GlobalStates.overviewOpen) {
                DaemonSocket.oskHide()
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
        active: root.oskActive
        onActiveChanged: {
            if (!oskLoader.active) {
                Ydotool.releaseAllKeys();
            }
        }

        sourceComponent: PanelWindow { // Window
            id: oskRoot
            visible: true
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
                DaemonSocket.oskDismiss()
            }
            exclusiveZone: root.pinned ? implicitHeight - Appearance.sizes.hyprlandGapsOut : 0
            implicitWidth: oskBackground.width + Appearance.sizes.elevationMargin * 2
            implicitHeight: oskBackground.height + Appearance.sizes.elevationMargin * 2
            WlrLayershell.namespace: "quickshell:osk"
            WlrLayershell.layer: WlrLayer.Overlay
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
                                downAction: () => {
                                    root.pinned = !root.pinned
                                    if (root.pinned) DaemonSocket.oskPin()
                                    else DaemonSocket.oskUnpin()
                                }
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
            DaemonSocket.oskToggle()
        }

        function close(): void {
            DaemonSocket.oskDismiss()
        }

        function open(): void {
            DaemonSocket.oskShow()
        }
    }

    GlobalShortcut {
        name: "oskToggle"
        description: "Toggles on screen keyboard on press"

        onPressed: {
            DaemonSocket.oskToggle()
        }
    }

    GlobalShortcut {
        name: "oskOpen"
        description: "Opens on screen keyboard on press"

        onPressed: {
            DaemonSocket.oskShow()
        }
    }

    GlobalShortcut {
        name: "oskClose"
        description: "Closes on screen keyboard on press"

        onPressed: {
            DaemonSocket.oskDismiss()
        }
    }

}
