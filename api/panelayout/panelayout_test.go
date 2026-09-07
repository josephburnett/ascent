package panelayout

import (
	"errors"
	"testing"
)

// TestTextFocusIDs pins the answer both sides of the ephemeral reap read:
// every leaf's TextFocus, in tree order, empty ones skipped.
func TestTextFocusIDs(t *testing.T) {
	blob := []byte(`{"v":1,"root":{"split":{"dir":"v","ratio":0.5,` +
		`"a":{"pane":{"id":"p1","anchor":"u/1","cx":0.5,"cy":0.5,"zoom":1,"text_focus":"u/7"}},` +
		`"b":{"split":{"dir":"h","ratio":0.5,` +
		`"a":{"pane":{"id":"p2","anchor":"u/1","cx":0.5,"cy":0.5,"zoom":1}},` +
		`"b":{"pane":{"id":"p3","anchor":"u/1","cx":0.5,"cy":0.5,"zoom":1,"text_focus":"u/9"}}}}}},"focus":"p1"}`)
	got, err := TextFocusIDs(blob)
	if err != nil {
		t.Fatalf("TextFocusIDs: %v", err)
	}
	if len(got) != 2 || got[0] != "u/7" || got[1] != "u/9" {
		t.Errorf("TextFocusIDs = %v, want [u/7 u/9]", got)
	}
}

// TestTextFocusIDsReadsTheProjectionNotThePlace pins which field decides
// when a leaf's Place stack also names a content frame. A second derivation
// would let one reader reap what the other protects.
func TestTextFocusIDsReadsTheProjectionNotThePlace(t *testing.T) {
	blob := []byte(`{"v":1,"root":{"pane":{"id":"p1","cx":0,"cy":0,"zoom":1,` +
		`"place":[{"g":"u/1"},{"d":"u/7","c":true}]}},"focus":"p1"}`)
	got, err := TextFocusIDs(blob)
	if err != nil {
		t.Fatalf("TextFocusIDs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("TextFocusIDs = %v, want none: text_focus is the one owner", got)
	}
}

// A blob from a newer Gridwell is a version error, so both readers do
// nothing instead of acting on a layout they half understand.
func TestTextFocusIDsRejectsANewerVersion(t *testing.T) {
	if _, err := TextFocusIDs([]byte(`{"v":999,"root":{}}`)); !errors.Is(err, ErrLayoutVersion) {
		t.Errorf("err = %v, want ErrLayoutVersion", err)
	}
	if _, err := TextFocusIDs([]byte(`not json`)); err == nil {
		t.Error("corrupt blob decoded without error")
	}
}
