package hyprparse

import "testing"

func TestHyprMergeNoConflict(t *testing.T) {
	orig := []byte("gaps_in = 4\nborder_size = 2\n")
	current := []byte("gaps_in = 8\nborder_size = 2\n")
	upstream := []byte("gaps_in = 4\nborder_size = 5\n")

	result, conflicts, err := ThreeWayMerge(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	doc := Parse(result)
	m := TopLevelKeyValue(doc)
	if m["gaps_in"] != "8" {
		t.Errorf("expected gaps_in=8 (user change), got %q", m["gaps_in"])
	}
	if m["border_size"] != "5" {
		t.Errorf("expected border_size=5 (upstream change), got %q", m["border_size"])
	}
}

func TestHyprMergeUserAddedBind(t *testing.T) {
	orig := []byte("gaps_in = 4\n")
	current := []byte("gaps_in = 4\nbind = Super, T, exec, kitty\n")
	upstream := []byte("gaps_in = 10\n")

	result, conflicts, err := ThreeWayMerge(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	doc := Parse(result)
	m := TopLevelKeyValue(doc)
	if m["gaps_in"] != "10" {
		t.Errorf("expected gaps_in=10 (upstream), got %q", m["gaps_in"])
	}
	binds := TopLevelBinds(doc)
	if len(binds) == 0 {
		t.Error("expected user's bind to be preserved")
	}
}

func TestHyprMergeSectionAdded(t *testing.T) {
	orig := []byte("gaps_in = 4\n")
	current := []byte("gaps_in = 4\n")
	upstream := []byte("gaps_in = 4\ngeneral {\n\tnew_setting = 1\n}\n")

	result, conflicts, err := ThreeWayMerge(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	doc := Parse(result)
	sec := FindSection(doc, "general")
	if sec == nil {
		t.Fatal("expected 'general' section from upstream")
	}
	m := SectionKeyValueMap(sec)
	if m["new_setting"] != "1" {
		t.Errorf("expected new_setting=1, got %q", m["new_setting"])
	}
}

func TestHyprMergeConflict(t *testing.T) {
	orig := []byte("gaps_in = 4\n")
	current := []byte("gaps_in = 8\n")
	upstream := []byte("gaps_in = 16\n")

	_, conflicts, err := ThreeWayMerge(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) == 0 {
		t.Error("expected a conflict on gaps_in")
	}
	found := false
	for _, c := range conflicts {
		if c == "gaps_in" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'gaps_in' in conflicts, got %v", conflicts)
	}
}

func TestHyprMergeKeepUserSection(t *testing.T) {
	orig := []byte("gaps_in = 4\n")
	current := []byte("gaps_in = 4\nmy_custom {\n\tsetting = 1\n}\n")
	upstream := []byte("gaps_in = 10\n")

	result, conflicts, err := ThreeWayMerge(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	doc := Parse(result)
	sec := FindSection(doc, "my_custom")
	if sec == nil {
		t.Fatal("expected user's 'my_custom' section to be preserved")
	}
	m := SectionKeyValueMap(sec)
	if m["setting"] != "1" {
		t.Errorf("expected setting=1, got %q", m["setting"])
	}
}
