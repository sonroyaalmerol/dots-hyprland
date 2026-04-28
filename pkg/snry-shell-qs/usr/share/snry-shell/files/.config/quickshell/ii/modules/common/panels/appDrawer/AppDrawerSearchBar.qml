pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs
import qs.services
import qs.modules.common
import qs.modules.common.widgets

Rectangle {
    id: root

    signal searchTextChanged(string text)

    color: Appearance.colors.colSurfaceContainer
    radius: Appearance.rounding.normal

    implicitWidth: 400
    implicitHeight: 48

    RowLayout {
        anchors.fill: parent
        anchors.leftMargin: 16
        anchors.rightMargin: 16
        spacing: 12

        MaterialSymbol {
            text: "search"
            iconSize: Appearance.font.pixelSize.large
            color: Appearance.colors.colOnSurfaceVariant
            Layout.preferredWidth: 24
            Layout.alignment: Qt.AlignVCenter
        }

        TextField {
            id: searchField
            Layout.fillWidth: true
            Layout.fillHeight: true
            placeholderText: Translation.tr("Search apps...")
            placeholderTextColor: Appearance.colors.colOnSurfaceVariant
            color: Appearance.colors.colOnSurface
            font.pixelSize: Appearance.font.pixelSize.normal
            font.family: Appearance.font.family.main
            verticalAlignment: Text.AlignVCenter
            background: Rectangle { color: "transparent" }
            onTextChanged: root.searchTextChanged(text)
        }

        Rectangle {
            Layout.preferredWidth: 32
            Layout.preferredHeight: 32
            visible: searchField.text.length > 0
            radius: width / 2
            color: clearMouseArea.containsMouse ? Appearance.colors.colLayer1Hover : "transparent"

            MaterialSymbol {
                anchors.centerIn: parent
                text: "close"
                iconSize: Appearance.font.pixelSize.small
                color: Appearance.colors.colOnSurfaceVariant
            }

            MouseArea {
                id: clearMouseArea
                anchors.fill: parent
                hoverEnabled: true
                cursorShape: Qt.PointingHandCursor
                onClicked: searchField.text = ""
            }
        }
    }
}
