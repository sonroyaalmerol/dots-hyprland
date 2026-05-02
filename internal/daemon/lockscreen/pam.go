package lockscreen

import (
	"fmt"
	"log"

	"github.com/msteinert/pam/v2"
)

func authenticateWithPAM(username, password string) error {
	trans, err := pam.StartFunc("login", username, func(s pam.Style, msg string) (string, error) {
		switch s {
		case pam.PromptEchoOff:
			return password, nil
		case pam.PromptEchoOn:
			return username, nil
		case pam.ErrorMsg:
			log.Printf("[LOCKSCREEN] PAM error: %s", msg)
			return "", nil
		case pam.TextInfo:
			log.Printf("[LOCKSCREEN] PAM info: %s", msg)
			return "", nil
		default:
			return "", fmt.Errorf("unsupported PAM message style: %v", s)
		}
	})
	if err != nil {
		return fmt.Errorf("PAM start failed: %w", err)
	}
	defer trans.End()

	err = trans.Authenticate(0)
	if err != nil {
		return fmt.Errorf("PAM authentication failed: %w", err)
	}

	err = trans.AcctMgmt(0)
	if err != nil {
		return fmt.Errorf("PAM account check failed: %w", err)
	}

	return nil
}
