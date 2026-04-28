pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Quickshell
import qs
import qs.services
import qs.modules.common
import qs.modules.common.widgets

Item {
    id: root

    signal closed()

    property var pinnedEntries: {
        const pinnedIds = Config.options.launcher.pinnedApps;
        return pinnedIds.map(id => DesktopEntries.byId(id)).filter(Boolean);
    }

    property var allEntries: {
        const all = AppSearch.list;
        return [...all].sort((a, b) => a.name.localeCompare(b.name));
    }

    property var filteredEntries: allEntries
    property string searchText: ""

    focus: true
    anchors.fill: parent

    Rectangle {
        anchors.fill: parent
        color: "#000000"
        opacity: Config.options.appDrawer.scrimOpacity
    }

    ColumnLayout {
        anchors.fill: parent
        anchors.topMargin: 40
        anchors.bottomMargin: 40
        anchors.leftMargin: 60
        anchors.rightMargin: 60
        spacing: 20

        // Search bar (optional)
        Loader {
            id: searchBarLoader
            Layout.fillWidth: true
            Layout.preferredHeight: visible ? 48 : 0
            active: Config.options.appDrawer.showSearchBar
            visible: active
            sourceComponent: AppDrawerSearchBar {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                onSearchTextChanged: function(text) {
                    root.searchText = text;
                    if (text.length > 0) {
                        root.filteredEntries = AppSearch.fuzzyQuery(text);
                    } else {
                        root.filteredEntries = root.allEntries;
                    }
                }
            }
        }

        // Pinned section
        RowLayout {
            Layout.fillWidth: true
            Layout.preferredHeight: visible ? implicitHeight : 0
            visible: pinnedEntries.length > 0 && root.searchText === ""
            spacing: 12

            StyledText {
                text: "Pinned"
                color: Appearance.colors.colOnSurface
                font.pixelSize: Appearance.font.pixelSize.normal
                font.bold: true
            }

            Item { Layout.fillWidth: true; Layout.minimumHeight: 1 }
        }

        Loader {
            Layout.fillWidth: true
            Layout.preferredHeight: visible ? implicitHeight : 0
            active: pinnedEntries.length > 0 && root.searchText === ""
            visible: active
            sourceComponent: AppDrawerGrid {
                anchors.left: parent.left
                anchors.right: parent.right
                desktopEntries: pinnedEntries
                gridColumns: Config.options.appDrawer.columns
            }
        }

        // All apps section
        RowLayout {
            Layout.fillWidth: true
            Layout.preferredHeight: visible ? implicitHeight : 0
            visible: root.searchText === ""
            spacing: 12

            StyledText {
                text: "All apps"
                color: Appearance.colors.colOnSurface
                font.pixelSize: Appearance.font.pixelSize.normal
                font.bold: true
            }

            Item { Layout.fillWidth: true; Layout.minimumHeight: 1 }
        }

        StyledFlickable {
            Layout.fillWidth: true
            Layout.fillHeight: true
            Layout.topMargin: root.searchText !== "" ? 0 : 10
            clip: true
            contentWidth: availableWidth
            contentHeight: allAppsGrid.implicitHeight

            AppDrawerGrid {
                id: allAppsGrid
                width: parent.width
                desktopEntries: root.filteredEntries
                gridColumns: Config.options.appDrawer.columns
            }
        }
    }

    Keys.onPressed: function(event) {
        if (event.key === Qt.Key_Escape) {
            root.closed();
        }
    }
}