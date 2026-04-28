package lockscreen

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// authenticate tries PAM first, then falls back to su.
func authenticate(password string) error {
	username := currentUser()
	log.Printf("[LOCKSCREEN] attempting PAM authentication for user %q", username)

	if err := authenticateWithPAM(username, password); err == nil {
		log.Printf("[LOCKSCREEN] PAM authentication succeeded for %q", username)
		return nil
	} else {
		log.Printf("[LOCKSCREEN] PAM authentication failed: %v", err)
	}

	log.Printf("[LOCKSCREEN] falling back to su authentication")
	if err := authenticateWithSu(username, password); err == nil {
		log.Printf("[LOCKSCREEN] su authentication succeeded for %q", username)
		return nil
	} else {
		log.Printf("[LOCKSCREEN] su failed: %v", err)
	}

	log.Printf("[LOCKSCREEN] all auth methods failed for %q", username)
	return fmt.Errorf("authentication failed")
}

// IsKeyringUnlocked checks if the keyring is already accessible
// (e.g. unlocked by PAM/pam_gnome_keyring.so during login).
func IsKeyringUnlocked() bool {
	cmd := exec.Command("secret-tool", "lookup", "service", "snry-shell")
	if cmd.Run() == nil {
		return true
	}
	// Check if the login keyring collection is unlocked via D-Bus
	cmd = exec.Command("busctl", "--user", "get-property", "org.freedesktop.secrets",
		"/org/freedesktop/secrets/collection/login", "org.freedesktop.Secret.Collection", "Locked")
	out, err := cmd.Output()
	if err == nil && strings.Contains(string(out), "false") {
		return true
	}
	return false
}

func unlockKeyring(password string) {
	cmd := exec.Command("secret-tool", "store", "--label=snry-shell", "service", "snry-shell")
	cmd.Stdin = strings.NewReader(password)
	if err := cmd.Run(); err == nil {
		exec.Command("secret-tool", "clear", "service", "snry-shell").Run()
		log.Printf("[LOCKSCREEN] keyring unlocked successfully")
		return
	}

	cmd = exec.Command("gnome-keyring-daemon", "--unlock")
	cmd.Stdin = strings.NewReader(password + "\n")
	if err := cmd.Run(); err == nil {
		log.Printf("[LOCKSCREEN] GNOME keyring unlocked via daemon")
		return
	}

	cmd = exec.Command("dbus-send", "--session", "--dest=org.gnome.keyring", "--type=method_call",
		"/org/freedesktop/portal/desktop", "org.freedesktop.portal.Settings.Read", "string:org.freedesktop.appearance", "string:color-scheme")
	if err := cmd.Run(); err == nil {
		log.Printf("[LOCKSCREEN] GNOME keyring unlocked via D-Bus")
		return
	}

	log.Printf("[LOCKSCREEN] could not unlock keyring (may not be running or not supported)")
}

func currentUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("LOGNAME"); u != "" {
		return u
	}
	return "user"
}
