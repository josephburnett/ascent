package panestate

import "testing"

func TestNewIsEmpty(t *testing.T) {
	if s := New(); s.Selected != "" {
		t.Fatalf("fresh state = %+v, want empty selection", s)
	}
}
