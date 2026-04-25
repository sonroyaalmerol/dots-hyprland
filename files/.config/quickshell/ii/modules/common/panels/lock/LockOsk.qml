import QtQuick
import QtQuick.Layouts
import qs.modules.common
import qs.modules.common.widgets
import "../../ii/onScreenKeyboard/layouts.js" as Layouts
pragma ComponentBehavior: Bound

Item {
    id: root
    required property LockContext context
    property bool showOsk: false
    property int shiftMode: 0
    property real baseWidth: 45
    property real baseHeight: 45
    property var layouts: Layouts.byName
    property var activeLayoutName: (layouts.hasOwnProperty(Config.options?.osk.layout))
        ? Config.options?.osk.layout
        : Layouts.defaultLayout
    property var currentLayout: layouts[activeLayoutName]

    opacity: showOsk ? 1 : 0
    y: showOsk ? 0 : 20
    Behavior on opacity {
        NumberAnimation {
            duration: Appearance.animation.elementMove.Duration
            easing.type: Appearance.animation.elementMove.type
        }
    }
    Behavior on y {
        NumberAnimation {
            duration: Appearance.animation.elementMove.Duration
            easing.type: Appearance.animation.elementMove.type
        }
    }

    property var widthMultiplier: ({
        "normal": 1,
        "fn": 1,
        "tab": 1.6,
        "caps": 1.9,
        "shift": 2.5,
        "control": 1.3,
        "space": 6,
        "expand": 2
    })
    property var heightMultiplier: ({
        "normal": 1,
        "fn": 0.7,
        "tab": 1,
        "caps": 1,
        "shift": 1,
        "control": 1
    })

    Timer {
        id: capsLockTimer
        property bool hasStarted: false
        property bool canCaps: false
        interval: 300
        onTriggered: {
            canCaps = false;
        }
    }

    Rectangle {
        id: keyboardContainer
        anchors {
            horizontalCenter: parent.horizontalCenter
            bottom: parent.bottom
        }
        width: keyboardLayout.implicitWidth + 20
        height: keyboardLayout.implicitHeight + 20
        color: Appearance.colors.colLayer0
        radius: Appearance.rounding.windowRounding

        ColumnLayout {
            id: keyboardLayout
            anchors.centerIn: parent
            spacing: 5

            Repeater {
                model: root.currentLayout.keys

                delegate: RowLayout {
                    id: keyRow
                    required property var modelData
                    spacing: 5

                    Repeater {
                        model: modelData
                        delegate: lockKey
                    }
                }
            }
        }
    }

    component lockKey: MouseArea {
        id: keyRoot
        required property var modelData
        property string label: modelData.label
        property string labelShift: modelData.labelShift || ""
        property string labelCaps: modelData.labelCaps || ""
        property string shape: modelData.shape
        property string keytype: modelData.keytype || "normal"
        property bool isBackspace: (label.toLowerCase() === "backspace")
        property bool isEnter: (label.toLowerCase() === "enter" || label.toLowerCase() === "return")
        property bool isShift: (shape === "shift" || shape === "expand")
        property bool isSpace: (shape === "space")
        property bool isSpacer: (keytype === "spacer")

        width: isSpacer ? 5 : (baseWidth * (widthMultiplier[shape] || 1))
        height: isSpacer ? baseHeight : (baseHeight * (heightMultiplier[shape] || 1))
        enabled: !isSpacer && shape !== "empty"

        visible: shape !== "empty" && !isSpacer

        onPressed: mouse => {
            context.resetClearTimer();

            if (isShift) {
                if (shiftMode === 0) {
                    shiftMode = 1;
                } else if (shiftMode === 1) {
                    if (!capsLockTimer.hasStarted) {
                        capsLockTimer.hasStarted = true;
                        capsLockTimer.canCaps = true;
                        capsLockTimer.start();
                    } else {
                        if (capsLockTimer.canCaps) {
                            shiftMode = 2;
                        } else {
                            shiftMode = 0;
                        }
                    }
                } else if (shiftMode === 2) {
                    shiftMode = 0;
                }
            } else if (isBackspace) {
                if (context.currentText.length > 0) {
                    context.currentText = context.currentText.slice(0, -1);
                }
            } else if (isEnter) {
                context.tryUnlock();
            } else if (isSpace) {
                context.currentText += " ";
            } else if (keytype === "normal" && label.length === 1) {
                let char = shiftMode === 0 ? label : (labelShift || label);
                context.currentText += char;
                if (shiftMode === 1) {
                    shiftMode = 0;
                }
            }
        }

        Rectangle {
            anchors.fill: parent
            radius: Appearance.rounding.small
            color: {
                if (isShift) {
                    return (root.shiftMode === 1 || root.shiftMode === 2)
                        ? Appearance.colors.colPrimary
                        : Appearance.colors.colLayer1;
                }
                return keyRoot.down
                    ? Appearance.colors.colLayer1Hover
                    : Appearance.colors.colLayer1;
            }
            Behavior on color {
                ColorAnimation {
                    duration: 100
                }
            }

            StyledText {
                anchors.fill: parent
                font.family: (isBackspace || isEnter) ? Appearance.font.family.iconMaterial : Appearance.font.family.main
                font.pixelSize: shape === "fn" ? Appearance.font.pixelSize.small :
                    (isBackspace || isEnter) ? Appearance.font.pixelSize.huge :
                    (isShift && labelCaps) ? Appearance.font.pixelSize.small :
                    Appearance.font.pixelSize.large
                horizontalAlignment: Text.AlignHCenter
                verticalAlignment: Text.AlignVCenter
                color: {
                    if (isShift) {
                        return (root.shiftMode === 1 || root.shiftMode === 2)
                            ? Appearance.colors.colOnPrimary
                            : Appearance.colors.colOnLayer1;
                    }
                    return Appearance.colors.colOnLayer1;
                }
                text: isBackspace ? "backspace" : isEnter ? "subdirectory_arrow_left" :
                    root.shiftMode === 2 ? (labelCaps || labelShift || label) :
                    root.shiftMode === 1 ? (labelShift || label) :
                    label
            }
        }
    }

    MouseArea {
        id: closeButton
        anchors {
            top: keyboardLayout.top
            right: keyboardLayout.right
            margins: -8
        }
        width: 36
        height: 36
        onClicked: root.showOsk = false

        Rectangle {
            anchors.fill: parent
            radius: width / 2
            color: closeButton.down ? Appearance.colors.colLayer1Hover : Appearance.colors.colLayer1

            MaterialSymbol {
                anchors.centerIn: parent
                iconSize: 20
                text: "keyboard_hide"
                color: Appearance.colors.colOnLayer1
            }
        }
    }
}
