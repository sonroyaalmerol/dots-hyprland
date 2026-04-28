import qs
import qs.services
import qs.modules.common
import qs.modules.common.widgets
import qs.modules.common.panels.appDrawer
import Qt.labs.synchronizer
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import Quickshell
import Quickshell.Io
import Quickshell.Wayland
import Quickshell.Hyprland

Scope {
    id: overviewScope
    property bool dontAutoCancelSearch: false

    PanelWindow {
        id: panelWindow
        property string searchingText: ""
        readonly property HyprlandMonitor monitor: Hyprland.monitorFor(panelWindow.screen)
        property bool monitorIsFocused: (Hyprland.focusedMonitor?.id == monitor?.id)
        visible: GlobalStates.overviewOpen

        WlrLayershell.namespace: "quickshell:overview"
        WlrLayershell.layer: WlrLayer.Top
        WlrLayershell.keyboardFocus: GlobalStates.overviewOpen ? WlrKeyboardFocus.OnDemand : WlrKeyboardFocus.None
        color: "transparent"

        mask: Region {
            item: GlobalStates.overviewOpen ? columnLayout : null
        }

        anchors {
            top: true
            bottom: true
            left: true
            right: true
        }

        Connections {
            target: GlobalStates
            function onOverviewOpenChanged() {
                if (!GlobalStates.overviewOpen) {
                    searchWidget.disableExpandAnimation();
                    overviewScope.dontAutoCancelSearch = false;
                    GlobalFocusGrab.dismiss();
                } else {
                    if (!overviewScope.dontAutoCancelSearch) {
                        searchWidget.cancelSearch();
                    }
                    GlobalFocusGrab.addDismissable(panelWindow);
                }
            }
        }

        Connections {
            target: GlobalFocusGrab
            function onDismissed() {
                GlobalStates.overviewOpen = false;
            }
        }
        implicitWidth: columnLayout.implicitWidth
        implicitHeight: columnLayout.implicitHeight

        function setSearchingText(text) {
            searchWidget.setSearchingText(text);
            searchWidget.focusFirstItem();
        }

        StyledFlickable {
            id: flickable
            visible: GlobalStates.overviewOpen
            anchors {
                horizontalCenter: parent.horizontalCenter
                top: parent.top
                bottom: parent.bottom
            }
            width: columnLayout.implicitWidth
            contentWidth: columnLayout.implicitWidth
            contentHeight: columnLayout.implicitHeight

            Keys.onPressed: event => {
                if (event.key === Qt.Key_Escape) {
                    GlobalStates.overviewOpen = false;
                } else if (event.key === Qt.Key_Left) {
                    if (!panelWindow.searchingText)
                        Hyprland.dispatch("workspace r-1");
                } else if (event.key === Qt.Key_Right) {
                    if (!panelWindow.searchingText)
                        Hyprland.dispatch("workspace r+1");
                }
            }

            Column {
                id: columnLayout
                spacing: -8

                SearchWidget {
                    id: searchWidget
                    anchors.horizontalCenter: parent.horizontalCenter
                    Synchronizer on searchingText {
                        property alias source: panelWindow.searchingText
                    }
                }

                Loader {
                    id: overviewLoader
                    anchors.horizontalCenter: parent.horizontalCenter
                    active: GlobalStates.overviewOpen && (Config?.options.overview.enable ?? true)
                    sourceComponent: OverviewWidget {
                        screen: panelWindow.screen
                        visible: (panelWindow.searchingText == "")
                    }
                }

                Loader {
                    id: appDrawerLoader
                    anchors.horizontalCenter: parent.horizontalCenter
                    active: GlobalStates.overviewOpen && panelWindow.searchingText === "" && (Config?.options.appDrawer.enabled ?? true)
                    visible: active && status === Loader.Ready
                    sourceComponent: Item {
                        implicitWidth: drawerCard.implicitWidth
                        implicitHeight: drawerCard.implicitHeight

                        StyledRectangularShadow { target: drawerCard }
                        Rectangle {
                            id: drawerCard
                            anchors.fill: parent
                            anchors.margins: Appearance.sizes.elevationMargin
                            property real padding: 16
                            implicitWidth: drawerColumn.implicitWidth + padding * 2
                            implicitHeight: drawerColumn.implicitHeight + padding * 2
                            radius: Appearance.rounding.large + padding
                            color: Appearance.colors.colBackgroundSurfaceContainer
                            clip: true

                            property var pinnedEntries: {
                                const pinnedIds = Config.options.launcher.pinnedApps;
                                return pinnedIds.map(id => DesktopEntries.byId(id)).filter(Boolean);
                            }
                            property var allEntries: {
                                const all = AppSearch.list;
                                return [...all].sort((a, b) => a.name.localeCompare(b.name));
                            }

                            ColumnLayout {
                                id: drawerColumn
                                anchors.centerIn: parent
                                spacing: 12

                                RowLayout {
                                    Layout.fillWidth: true
                                    visible: drawerCard.pinnedEntries.length > 0
                                    StyledText {
                                        text: Translation.tr("Pinned")
                                        color: Appearance.colors.colOnSurface
                                        font.pixelSize: Appearance.font.pixelSize.normal
                                        font.bold: true
                                    }
                                    Item { Layout.fillWidth: true }
                                }

                                AppDrawerGrid {
                                    visible: drawerCard.pinnedEntries.length > 0
                                    desktopEntries: drawerCard.pinnedEntries
                                    gridColumns: Config.options.appDrawer.columns
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    StyledText {
                                        text: Translation.tr("All apps")
                                        color: Appearance.colors.colOnSurface
                                        font.pixelSize: Appearance.font.pixelSize.normal
                                        font.bold: true
                                    }
                                    Item { Layout.fillWidth: true }
                                }

                                AppDrawerGrid {
                                    id: allAppsGrid
                                    desktopEntries: drawerCard.allEntries
                                    gridColumns: Config.options.appDrawer.columns
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    function toggleClipboard() {
        if (GlobalStates.overviewOpen && overviewScope.dontAutoCancelSearch) {
            GlobalStates.overviewOpen = false;
            return;
        }
        overviewScope.dontAutoCancelSearch = true;
        panelWindow.setSearchingText(Config.options.search.prefix.clipboard);
        GlobalStates.overviewOpen = true;
    }

    function toggleEmojis() {
        if (GlobalStates.overviewOpen && overviewScope.dontAutoCancelSearch) {
            GlobalStates.overviewOpen = false;
            return;
        }
        overviewScope.dontAutoCancelSearch = true;
        panelWindow.setSearchingText(Config.options.search.prefix.emojis);
        GlobalStates.overviewOpen = true;
    }

    IpcHandler {
        target: "search"

        function toggle() {
            GlobalStates.overviewOpen = !GlobalStates.overviewOpen;
        }
        function workspacesToggle() {
            GlobalStates.overviewOpen = !GlobalStates.overviewOpen;
        }
        function close() {
            GlobalStates.overviewOpen = false;
        }
        function open() {
            GlobalStates.overviewOpen = true;
        }
        function toggleReleaseInterrupt() {
            GlobalStates.superReleaseMightTrigger = false;
        }
        function clipboardToggle() {
            overviewScope.toggleClipboard();
        }
    }

    GlobalShortcut {
        name: "searchToggle"
        description: "Toggles search on press"

        onPressed: {
            GlobalStates.overviewOpen = !GlobalStates.overviewOpen;
        }
    }
    GlobalShortcut {
        name: "overviewWorkspacesClose"
        description: "Closes overview on press"

        onPressed: {
            GlobalStates.overviewOpen = false;
        }
    }
    GlobalShortcut {
        name: "overviewWorkspacesToggle"
        description: "Toggles overview on press"

        onPressed: {
            GlobalStates.overviewOpen = !GlobalStates.overviewOpen;
        }
    }
    GlobalShortcut {
        name: "searchToggleRelease"
        description: "Toggles search on release"

        onPressed: {
            GlobalStates.superReleaseMightTrigger = true;
        }

        onReleased: {
            if (!GlobalStates.superReleaseMightTrigger) {
                GlobalStates.superReleaseMightTrigger = true;
                return;
            }
            GlobalStates.overviewOpen = !GlobalStates.overviewOpen;
        }
    }
    GlobalShortcut {
        name: "searchToggleReleaseInterrupt"
        description: "Interrupts possibility of search being toggled on release. " + "This is necessary because GlobalShortcut.onReleased in quickshell triggers whether or not you press something else while holding the key. " + "To make sure this works consistently, use binditn = MODKEYS, catchall in an automatically triggered submap that includes everything."

        onPressed: {
            GlobalStates.superReleaseMightTrigger = false;
        }
    }
    GlobalShortcut {
        name: "overviewClipboardToggle"
        description: "Toggle clipboard query on overview widget"

        onPressed: {
            overviewScope.toggleClipboard();
        }
    }

    GlobalShortcut {
        name: "overviewEmojiToggle"
        description: "Toggle emoji query on overview widget"

        onPressed: {
            overviewScope.toggleEmojis();
        }
    }
}
