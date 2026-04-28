package lockscreen

import (
	"fmt"
	"os/exec"
	"strings"
)

func authenticateWithSu(username, password string) error {
	cmd := exec.Command("su", username, "-c", "true")
	cmd.Stdin = strings.NewReader(password + "\n")
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("su auth failed: %w", err)
	}
	return nil
}
