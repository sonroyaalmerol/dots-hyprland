package dm

import (
	"testing"
)

func TestLookupUserIDs(t *testing.T) {
	// "root" should always exist in /etc/passwd.
	uid, gid, err := lookupUserIDs("root")
	if err != nil {
		t.Fatalf("lookupUserIDs(root): %v", err)
	}
	if uid != 0 {
		t.Errorf("root uid = %d, want 0", uid)
	}
	if gid != 0 {
		t.Errorf("root gid = %d, want 0", gid)
	}
}

func TestLookupUserIDsNonexistent(t *testing.T) {
	_, _, err := lookupUserIDs("nonexistent-user-xyz-12345")
	if err == nil {
		t.Error("lookupUserIDs should fail for nonexistent user")
	}
}

func TestLookupUserIDsNobody(t *testing.T) {
	// "nobody" should exist on most Linux systems.
	uid, _, err := lookupUserIDs("nobody")
	if err != nil {
		t.Skip("nobody user not found")
	}
	// nobody typically has a high UID (65534 on Linux).
	if uid == 0 {
		t.Error("nobody should not be uid 0")
	}
}
