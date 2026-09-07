package idshape

import (
	"strconv"
	"testing"
)

// TestNewShortIDShape pins the id shape: 7 characters, lowercase base36,
// leading letter, so an id never parses as an integer.
func TestNewShortIDShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewShortID()
		if len(id) != shortIDLen {
			t.Fatalf("len(%q) = %d, want %d", id, len(id), shortIDLen)
		}
		if id[0] < 'a' || id[0] > 'z' {
			t.Fatalf("%q does not start with a letter — it could be purely numeric", id)
		}
		for j := 0; j < len(id); j++ {
			c := id[j]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z')) {
				t.Fatalf("%q contains %q outside lowercase base36", id, c)
			}
		}
		if _, err := strconv.ParseInt(id, 10, 64); err == nil {
			t.Fatalf("%q parses as an integer — indistinguishable from a tile id", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q within 1000 draws", id)
		}
		seen[id] = true
	}
}

// TestNewUUIDStays128Bit pins that NewUUID keeps its full 128 bits. It
// mints system.plugin_uuid, which claims uniqueness across nodes.
func TestNewUUIDStays128Bit(t *testing.T) {
	id := NewUUID()
	if len(id) != 32 {
		t.Fatalf("NewUUID() = %q (len %d), want 32 hex chars", id, len(id))
	}
}

// The empty string is not a namespace segment: a nameless connection stanza
// would occupy it, so "<node>//12" would peel to it and every id through it
// would read as the node's own.
func TestValidateSegmentRefusesTheEmptySegment(t *testing.T) {
	if err := ValidateSegment("connection name", ""); err == nil {
		t.Fatal("an empty segment must be refused")
	}
}
