import qs.services
import QtQuick
import QtQuick.Layouts
import QtQuick.Controls
import Quickshell
import qs.modules.common
import qs.modules.common.widgets
import qs.modules.ii.overlay

OverlayBackground {
    id: root

    enum State { Normal, Success, Error }

    property real padding: 16
    property var currentState: FpsLimiterContent.State.Normal
    implicitWidth: content.implicitWidth + (padding * 2)
    implicitHeight: content.implicitHeight + (padding * 2)

    Timer {
        id: iconResetTimer
        interval: 1000
        onTriggered: {
            root.currentState = FpsLimiterContent.State.Normal;
        }
    }

    function applyLimit() {
        var fpsValue = parseInt(fpsField.text);
        if (isNaN(fpsValue) || fpsValue < 0) {
            root.currentState = FpsLimiterContent.State.Error;
            iconResetTimer.restart();
            fpsField.text = "";
            return;
        }
        DaemonSocket.fpsSet(fpsValue.toString())
        root.currentState = FpsLimiterContent.State.Success;
        iconResetTimer.restart();
        fpsField.text = "";
    }

    RowLayout {
        id: content
        anchors.centerIn: parent
        spacing: 4

        ToolbarTextField {
            id: fpsField
            Layout.fillWidth: true
            Layout.preferredWidth: 200
            placeholderText: root.currentState === FpsLimiterContent.State.Error ? Translation.tr("Enter a valid number") : Translation.tr("Set FPS limit")
            inputMethodHints: Qt.ImhDigitsOnly
            focus: true

            onAccepted: {
                root.applyLimit();
            }
        }

        IconToolbarButton {
            id: applyButton
            text: switch (root.currentState) {
                case FpsLimiterContent.State.Error: return "close";
                case FpsLimiterContent.State.Success: return "check";
                case FpsLimiterContent.State.Normal:
                default: return "save";
            }
            enabled: root.currentState === FpsLimiterContent.State.Normal && fpsField.text.length > 0
            onClicked: {
                root.applyLimit();
            }
        }
    }
}