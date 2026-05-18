package dm

import (
	"testing"
)

func TestVTCloseNilFD(t *testing.T) {
	vt := &VT{fd: nil}
	if err := vt.Close(); err != nil {
		t.Errorf("Close on nil fd should succeed: %v", err)
	}
}

func TestVTSetTextModeNilFD(t *testing.T) {
	vt := &VT{fd: nil}
	if err := vt.SetTextMode(); err != nil {
		t.Errorf("SetTextMode on nil fd should succeed: %v", err)
	}
}

func TestVTNum(t *testing.T) {
	vt := &VT{num: 7}
	if vt.Num() != 7 {
		t.Errorf("Num() = %d, want 7", vt.Num())
	}
}
