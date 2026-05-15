pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Effects
import Qt5Compat.GraphicalEffects
import Quickshell

import qs
import qs.modules.common
import qs.modules.common.functions
import qs.modules.common.utils
import qs.modules.common.widgets
import qs.services

Item {
    id: root

    property double scaleFactor: 1
    property color overlayColor: "#BB000000"
    property color textColor: "white"
    required property string screenshotPath

    readonly property string wikiLink: "https://ii.clsty.link/en/ii-qs/02usage/#setting-it-up"

    property bool loading: true
    property var visionParagraphs: []
    property list<string> translationKeys: []
    property var translation: ({})

    function translate(s: string): string {
        return translation[s] ?? s;
    }

    property bool error: false
    property string errorMessage: ""
    function showError() {
        error = true;
    }

    property real windowWidth: QsWindow.window.screen.width
    property real windowHeight: QsWindow.window.screen.height

    StyledImage {
        id: screenshotImage
        z: 1
        asynchronous: false
        width: root.windowWidth
        height: root.windowHeight
        sourceSize: Qt.size(root.windowWidth, root.windowHeight)
        source: Qt.resolvedUrl(root.screenshotPath)
        visible: false
    }

    Item {
        id: blurMaskItem
        z: 2
        width: root.windowWidth
        height: root.windowHeight
        layer.enabled: true
        visible: false
        Repeater {
            model: root.loading ? [] : root.visionParagraphs
            delegate: VisionBoundingBoxRect {
                readonly property string text: modelData.text
                readonly property string translatedText: root.translate(text)
                visible: translatedText != text
                scaleFactor: 1
            }
        }
    }

    MaskMultiEffect {
        z: 4
        implicitWidth: parent.width
        implicitHeight: parent.height
        width: parent.width
        height: parent.height

        // Mask
        source: screenshotImage
        maskSource: blurMaskItem

        // Blur
        blurEnabled: true
        blur: 1
        blurMax: 50
        blurMultiplier: root.scaleFactor
        autoPaddingEnabled: false
    }

    Item {
        id: textItems
        z: 999
        Repeater {
            model: root.loading ? [] : root.visionParagraphs
            delegate: TextItem {}
        }
    }

    component VisionBoundingBoxRect: Rectangle {
        required property var modelData
        property real scaleFactor: root.scaleFactor
        property list<var> boundingVertices: modelData.boundingBox.vertices
        property real unscaledX: boundingVertices[0].x
        property real unscaledY: boundingVertices[0].y
        property real unscaledWidth: boundingVertices[1].x - boundingVertices[0].x
        property real unscaledHeight: boundingVertices[3].y - boundingVertices[0].y

        // Calculate rotation based on first two vertices (top-left to top-right)
        property real dx: boundingVertices[1].x - boundingVertices[0].x
        property real dy: boundingVertices[1].y - boundingVertices[0].y
        transformOrigin: Item.TopLeft
        rotation: {
            var angle = Math.atan2(dy, dx) * 180 / Math.PI;
            return angle;
        }

        x: unscaledX * scaleFactor
        y: unscaledY * scaleFactor
        width: unscaledWidth * scaleFactor
        height: unscaledHeight * scaleFactor
        radius: 4
    }

    component TextItem: VisionBoundingBoxRect {
        id: ti
        readonly property string text: modelData.text
        readonly property string translatedText: root.translate(text)
        visible: translatedText != text

        color: ColorUtils.transparentize(Appearance.colors.colSecondaryContainer, 0.4)
        Behavior on color {
            animation: Appearance.animation.elementMoveFast.colorAnimation.createObject(this)
        }

        Loader {
            active: ti.visible
            sourceComponent: Component {
                Item {
                    Component.onCompleted: {
                        DaemonSocket.sendCommand("text-color --image " + root.screenshotPath
                            + " --crop-x " + Math.round(ti.unscaledX)
                            + " --crop-y " + Math.round(ti.unscaledY)
                            + " --crop-w " + Math.round(ti.unscaledWidth)
                            + " --crop-h " + Math.round(ti.unscaledHeight))
                    }
                    Connections {
                        target: DaemonSocket
                        function onTextColorResult(data) {
                            try {
                                var colorData = typeof data.result === 'string' ? JSON.parse(data.result) : data.result
                                ti.color = ColorUtils.transparentize(colorData.background, 0.4)
                                tiText.color = colorData.text
                            } catch (e) {}
                        }
                    }
                }
            }
        }

        SqueezedAnnotationStyledText {
            id: tiText
            width: parent.width
            height: parent.height
            text: ti.translatedText
            scaleFactor: root.scaleFactor

            Behavior on color {
                animation: Appearance.animation.elementMoveFast.colorAnimation.createObject(this)
            }
        }
    }
}