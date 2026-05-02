package sectionparse

import "testing"

func TestSectionMergeNoConflict(t *testing.T) {
	orig := []byte("# >>> snry-shell >>>\nexport PATH=/usr/bin\n# <<< snry-shell <<<\n")
	current := []byte("# >>> snry-shell >>>\nalias ll='ls -la'\nexport PATH=/usr/bin\n# <<< snry-shell <<<\n")
	upstream := []byte("# >>> snry-shell >>>\nexport PATH=/usr/local/bin:/usr/bin\n# <<< snry-shell <<<\n")

	result, conflicts, err := MergeSection(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	resultStr := string(result)
	if !HasMarker(result) {
		t.Error("expected markers in result")
	}
	if !containsStr(resultStr, "ll =") {
		t.Error("expected user's alias to be preserved")
	}
	if !containsStr(resultStr, "/usr/local/bin:/usr/bin") {
		t.Error("expected upstream PATH change to be applied")
	}
}

func TestSectionMergeConflict(t *testing.T) {
	orig := []byte("# >>> snry-shell >>>\nexport MY_VAR=original\n# <<< snry-shell <<<\n")
	current := []byte("# >>> snry-shell >>>\nexport MY_VAR=user_value\n# <<< snry-shell <<<\n")
	upstream := []byte("# >>> snry-shell >>>\nexport MY_VAR=upstream_value\n# <<< snry-shell <<<\n")

	_, conflicts, err := MergeSection(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) == 0 {
		t.Error("expected a conflict on MY_VAR")
	}
}

func TestSectionMergeNoMarker(t *testing.T) {
	// When orig and current have no markers, upstream content should be wrapped
	orig := []byte("some existing bashrc content\n")
	current := []byte("some existing bashrc content\n")
	upstream := []byte("export PATH=/usr/local/bin\n")

	result, conflicts, err := MergeSection(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}
	if !HasMarker(result) {
		t.Error("expected result to be wrapped in markers")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStrHelper(s, substr))
}

func containsStrHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
