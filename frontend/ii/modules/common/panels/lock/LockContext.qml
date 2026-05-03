import qs
import qs.services
import qs.modules.common
import QtQuick
import Quickshell
import Quickshell.Services.Pam

Scope {
    id: root

    enum ActionEnum { Unlock, Poweroff, Reboot }

    signal shouldReFocus()
    signal unlocked(targetAction: var)
    signal failed()

    // These properties are in the context and not individual lock surfaces
    // so all surfaces can share the same state.
    property string currentText: ""
    property bool unlockInProgress: false
    property bool showFailure: false
    property bool hasFingerprint: false
    property var targetAction: LockContext.ActionEnum.Unlock
    property bool alsoInhibitIdle: false

    // Lockout state from daemon
    property int remainingAttempts: 3
    property bool isLockedOut: false
    property int lockoutSeconds: 0

    function resetTargetAction() {
        root.targetAction = LockContext.ActionEnum.Unlock;
    }

    function clearText() {
        root.currentText = "";
    }

    function resetClearTimer() {
        passwordClearTimer.restart();
    }

    function reset() {
        root.resetTargetAction();
        root.clearText();
        root.unlockInProgress = false;
        root.remainingAttempts = 3;
        root.isLockedOut = false;
        root.lockoutSeconds = 0;
        root.stopFingerPam();
    }

    Timer {
        id: passwordClearTimer
        interval: 10000
        onTriggered: {
            root.reset();
        }
    }

    onCurrentTextChanged: {
        if (currentText.length > 0) {
            showFailure = false;
            GlobalStates.screenUnlockFailed = false;
        }
        GlobalStates.screenLockContainsCharacters = currentText.length > 0;
        passwordClearTimer.restart();
    }

    function tryUnlock(alsoInhibitIdle = false) {
        root.alsoInhibitIdle = alsoInhibitIdle;
        root.unlockInProgress = true;
        DaemonSocket.authenticate(root.currentText);
    }

    function tryFingerUnlock() {
        if (root.hasFingerprint) {
            fingerPam.start();
        }
    }

    function stopFingerPam() {
        if (fingerPam.active) {
            fingerPam.abort();
        }
    }

    Connections {
        target: DaemonSocket
        function onAuthResult(data) {
            root.unlockInProgress = false;
            if (data.success) {
                root.unlocked(root.targetAction);
                stopFingerPam();
            } else {
                root.clearText();
                root.remainingAttempts = data.remaining ?? 3;
                root.isLockedOut = data.lockedOut ?? false;
                GlobalStates.screenUnlockFailed = true;
                root.showFailure = true;
            }
        }
    }

    Connections {
        target: DaemonSocket
        function onLockoutTick(remainingSeconds) {
            root.lockoutSeconds = remainingSeconds;
            if (remainingSeconds <= 0) {
                root.isLockedOut = false;
            }
        }
    }

    Connections {
        target: DaemonSocket
        function onFprintdResult(available, enrolled) {
            root.hasFingerprint = available && enrolled
        }
    }

    Component.onCompleted: {
        DaemonSocket.fprintdCheck()
    }

    PamContext {
        id: fingerPam

        configDirectory: "pam"
        config: "fprintd.conf"

        onCompleted: result => {
            if (result == PamResult.Success) {
                root.unlocked(root.targetAction);
                stopFingerPam();
            } else if (result == PamResult.Error) { // if timeout or etc..
                tryFingerUnlock()
            }
        }
    }
}