pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import Quickshell
import Quickshell.Widgets
import qs
import qs.services
import qs.modules.common
import qs.modules.common.widgets

RippleButton {
    id: root
    required property var desktopEntry

    property bool pinnedStart: LauncherApps.isPinned(root.desktopEntry?.id ?? "")

    implicitWidth: 96
    implicitHeight: 90
    buttonRadius: Appearance.rounding.normal

    contentItem: ColumnLayout {
        spacing: 4
        anchors.horizontalCenter: parent.horizontalCenter
        IconImage {
            Layout.topMargin: 10
            Layout.alignment: Qt.AlignHCenter
            source: Quickshell.iconPath(root.desktopEntry?.icon ?? "application-x-executable")
            implicitSize: 48
        }
        StyledText {
            Layout.fillHeight: true
            Layout.fillWidth: true
            Layout.leftMargin: 6
            Layout.rightMargin: 6
            Layout.bottomMargin: 8
            text: root.desktopEntry?.name ?? ""
            wrapMode: Text.Wrap
            elide: Text.ElideRight
            maximumLineCount: 2
            horizontalAlignment: Text.AlignHCenter
            verticalAlignment: Text.AlignTop
            color: Appearance.colors.colOnSurface
            font.pixelSize: Appearance.font.pixelSize.small
        }
    }

    altAction: () => {
        appMenu.popup()
    }

    Menu {
        id: appMenu

        MenuItem {
            text: root.pinnedStart ? Translation.tr("Unpin from drawer") : Translation.tr("Pin to drawer")
            onTriggered: {
                if (root.desktopEntry?.id)
                    LauncherApps.togglePin(root.desktopEntry.id);
            }
        }
    }

    releaseAction: () => {
        if (root.desktopEntry?.id) {
            GlobalStates.appDrawerOpen = false;
            root.desktopEntry.execute();
        }
    }
}