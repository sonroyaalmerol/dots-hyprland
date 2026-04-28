pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.modules.common.widgets

GridLayout {
    id: root

    property list<var> desktopEntries: []
    property int gridColumns: 6

    columnSpacing: 0
    rowSpacing: 0

    uniformCellHeights: true
    uniformCellWidths: true

    columns: root.gridColumns

    Repeater {
        model: root.desktopEntries
        delegate: AppDrawerButton {
            id: appButton
            required property var modelData
            desktopEntry: modelData
        }
    }
}