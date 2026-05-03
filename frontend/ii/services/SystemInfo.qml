pragma Singleton
pragma ComponentBehavior: Bound

import qs.services
import QtQuick
import Quickshell

Singleton {
	id: root
	property string distroName: DaemonSocket.systemDistroName
	property string distroId: DaemonSocket.systemDistroId
	property string distroIcon: DaemonSocket.systemDistroIcon
	property string username: DaemonSocket.systemUsername
	property string homeUrl: DaemonSocket.systemHomeUrl
	property string documentationUrl: DaemonSocket.systemDocumentationUrl
	property string supportUrl: DaemonSocket.systemSupportUrl
	property string bugReportUrl: DaemonSocket.systemBugReportUrl
	property string privacyPolicyUrl: DaemonSocket.systemPrivacyPolicyUrl
	property string logo: DaemonSocket.systemLogo
	property string desktopEnvironment: DaemonSocket.systemDesktopEnvironment
	property string windowingSystem: DaemonSocket.systemWindowingSystem
}
