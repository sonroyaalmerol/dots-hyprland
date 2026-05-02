import QtQuick
import qs
import qs.services
import qs.modules.common
import qs.modules.common.widgets

RippleButton {
	id: root

	property real buttonPadding: 5
	implicitWidth: distroIcon.width + buttonPadding * 2
	implicitHeight: distroIcon.height + buttonPadding * 2
	buttonRadius: Appearance.rounding.full
	colBackgroundHover: Appearance.colors.colLayer1Hover
	colRipple: Appearance.colors.colLayer1Active
	colBackgroundToggled: Appearance.colors.colSecondaryContainer
	colBackgroundToggledHover: Appearance.colors.colSecondaryContainerHover
	colRippleToggled: Appearance.colors.colSecondaryContainerActive
	toggled: GlobalStates.overviewOpen

	onPressed: {
		GlobalStates.overviewOpen = !GlobalStates.overviewOpen;
	}

	CustomIcon {
		id: distroIcon
		anchors.centerIn: parent
		width: 19.5
		height: 19.5
		source: Config.options.bar.topLeftIcon == 'distro' ? SystemInfo.distroIcon : `${Config.options.bar.topLeftIcon}-symbolic`
		colorize: true
		color: Appearance.colors.colOnLayer0
	}
}