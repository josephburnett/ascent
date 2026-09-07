package textedit

import "testing"

func TestSaveClaim(t *testing.T) {
	cases := []struct {
		name           string
		rowOwnsContent bool
		rowVersion     int64
		basis          int64
		haveBasis      bool
		want           int64
	}{
		{"owner row, basis", true, 7, 5, true, 5},
		{"link row, basis", false, 7, 5, true, 5},
		// With no basis the row snapshot stands in, and only when that row
		// owns the bytes.
		{"owner row, no basis", true, 7, 0, false, 7},
		{"link row, no basis", false, 7, 0, false, 0},
		{"no row, no basis", false, 0, 0, false, 0},
	}
	for _, c := range cases {
		if got := SaveClaim(c.rowOwnsContent, c.rowVersion, c.basis, c.haveBasis); got != c.want {
			t.Errorf("%s: SaveClaim = %d, want %d", c.name, got, c.want)
		}
	}
}

// A refused save's reconcile drops the content entry and refetches a foreign
// writer's row at version 6, while the flush behind it holds bytes read at 5.
// Claiming 6 would vouch for bytes this client never saw.
func TestSaveClaimFallbackIsWhatTheFlushSaw(t *testing.T) {
	const sawAtFlush, foreignWriterRow = 5, 6
	if got := SaveClaim(true, sawAtFlush, 0, false); got != sawAtFlush {
		t.Errorf("fallback claimed %d, want the snapshot %d", got, sawAtFlush)
	}
	if got := SaveClaim(true, foreignWriterRow, 0, false); got == sawAtFlush {
		t.Fatal("SaveClaim must return what it is given: the caller owes it the snapshot")
	}
}
