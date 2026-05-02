package kvparse

import "testing"

func TestMergeKVNoConflict(t *testing.T) {
	orig := []byte("a=1\nb=2\n")
	current := []byte("a=99\nb=2\n")
	upstream := []byte("a=1\nb=20\n")

	result, conflicts, err := MergeKV(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	m := Parse(result).AllKeyValueMap()
	if trim(m["a"]) != "99" {
		t.Errorf("expected a=99 (user change), got %q", m["a"])
	}
	if trim(m["b"]) != "20" {
		t.Errorf("expected b=20 (upstream change), got %q", m["b"])
	}
}

func TestMergeKVUserOnly(t *testing.T) {
	orig := []byte("key=old\n")
	current := []byte("key=new\n")
	upstream := []byte("key=old\n")

	result, conflicts, err := MergeKV(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	m := Parse(result).AllKeyValueMap()
	if trim(m["key"]) != "new" {
		t.Errorf("expected user's value 'new', got %q", m["key"])
	}
}

func TestMergeKVUpstreamOnly(t *testing.T) {
	orig := []byte("key=old\n")
	current := []byte("key=old\n")
	upstream := []byte("key=updated\n")

	result, conflicts, err := MergeKV(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	m := Parse(result).AllKeyValueMap()
	if trim(m["key"]) != "updated" {
		t.Errorf("expected upstream value 'updated', got %q", m["key"])
	}
}

func TestMergeKVConflict(t *testing.T) {
	orig := []byte("key=original\n")
	current := []byte("key=user_change\n")
	upstream := []byte("key=upstream_change\n")

	_, conflicts, err := MergeKV(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) == 0 {
		t.Error("expected a conflict")
	}
	found := false
	for _, c := range conflicts {
		if c == "key" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected conflict on 'key', got %v", conflicts)
	}
}

func TestMergeKVNewKey(t *testing.T) {
	orig := []byte("a=1\n")
	current := []byte("a=1\n")
	upstream := []byte("a=1\nb=2\n")

	result, conflicts, err := MergeKV(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	m := Parse(result).AllKeyValueMap()
	if trim(m["b"]) != "2" {
		t.Errorf("expected new key b=2, got %q", m["b"])
	}
}

func TestMergeKVDeletedKey(t *testing.T) {
	orig := []byte("a=1\nb=2\n")
	current := []byte("a=1\n")
	upstream := []byte("a=1\nb=2\n")

	result, conflicts, err := MergeKV(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	m := Parse(result).AllKeyValueMap()
	if _, exists := m["b"]; exists {
		t.Error("expected key 'b' to be deleted")
	}
}

func TestMergeKVNoChanges(t *testing.T) {
	orig := []byte("key=val\n")
	current := []byte("key=val\n")
	upstream := []byte("key=val\n")

	result, conflicts, err := MergeKV(orig, current, upstream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	m := Parse(result).AllKeyValueMap()
	if trim(m["key"]) != "val" {
		t.Errorf("expected 'val', got %q", m["key"])
	}
}
