package cache

import (
	"slices"
	"testing"

	"github.com/josephburnett/gridwell/api/rpc"
)

// The node as a client sees it: its own home, one local plugin, one
// connection, and behind that connection the far node's home and one of the
// far node's plugins. Health uuids gain one segment per hop exactly as ids
// do, so every source below names a chain that its grids' ids start with.
const (
	node    = "n1abcde"
	fsPl    = "fs9xyzw"
	conn    = node + "/laptop"
	farHome = conn + "/far9xyz"
	farPl   = conn + "/rp9plug"
)

// seedSources caches one grid per source, each id shaped as the wire
// qualifies it.
func seedSources(t *testing.T) *Cache {
	t.Helper()
	c := New()
	for _, id := range []string{
		node + "/7",    // this node's home store
		fsPl + "/1",    // a local plugin
		farHome + "/1", // the far node's home, through the connection
		farPl + "/4",   // a far node's plugin, one hop deeper still
		fsPl + "/12",   // a second grid of the local plugin
	} {
		c.PutGrid(rpc.Grid{ID: id}, nil)
	}
	return c
}

// The narrowing itself. One source flapping is news about that source's
// grids and nothing else, so refetching another source's rooms spends round
// trips on answers nobody had reason to doubt.
func TestAFlapResyncsOnlyTheGridsItsSourceServes(t *testing.T) {
	c := seedSources(t)

	got := c.ResyncSet(fsPl)
	want := []string{fsPl + "/1", fsPl + "/12"}
	if !slices.Equal(got, want) {
		t.Fatalf("ResyncSet(%q) = %v, want %v", fsPl, got, want)
	}
	for _, id := range got {
		if !ServedBy(id, fsPl) {
			t.Errorf("%s is in the plugin's resync set but not served by it", id)
		}
	}
	// The other sources' grids are untouched: nothing about them changed.
	for _, other := range []string{node + "/7", farHome + "/1", farPl + "/4"} {
		if slices.Contains(got, other) {
			t.Errorf("one plugin's flap resynced %s, a grid it does not serve", other)
		}
	}
}

// A source is a CHAIN, so a connection's flap owns everything reachable only
// through it — the far node's home and the far node's plugins alike. String
// equality on the owning namespace would resync neither.
func TestAConnectionsFlapOwnsEveryGridChainedThroughIt(t *testing.T) {
	c := seedSources(t)

	got := c.ResyncSet(conn)
	want := []string{farHome + "/1", farPl + "/4"}
	if !slices.Equal(got, want) {
		t.Fatalf("ResyncSet(%q) = %v, want %v", conn, got, want)
	}

	// And one hop deeper: the far node's own home store flapping is news
	// about its grids only, not about the far node's other plugins.
	got = c.ResyncSet(farHome)
	want = []string{farHome + "/1"}
	if !slices.Equal(got, want) {
		t.Fatalf("ResyncSet(%q) = %v, want %v", farHome, got, want)
	}
}

// The paths that are not a per-source flap keep their breadth: a stream gap
// has no cursor and names no source, so the whole cache is the answer.
func TestEverySourceIsTheWholeCache(t *testing.T) {
	c := seedSources(t)

	got := c.ResyncSet(EverySource)
	if len(got) != 5 {
		t.Fatalf("ResyncSet(EverySource) = %v, want all 5 cached grids", got)
	}
	if !slices.Equal(got, c.KnownGridIDs()) {
		t.Errorf("ResyncSet(EverySource) = %v, KnownGridIDs = %v; they are one fact", got, c.KnownGridIDs())
	}
}

// The predicate, on the boundaries a prefix test can get wrong. It answers
// for tile ids too, because the latches and the in-flight claims a flap
// clears are keyed by tile id.
func TestServedBy(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		source string
		want   bool
	}{
		{"a local plugin's grid", fsPl + "/1", fsPl, true},
		{"a local plugin's key-form tile", fsPl + "/" + rpc.KeyTileID("/home/joe"), fsPl, true},
		{"another plugin's grid", "zz9zzzz/1", fsPl, false},
		{"a segment that merely starts the same", fsPl + "x/1", fsPl, false},
		{"the source's own name is not a grid it serves", fsPl, fsPl, false},
		{"the far node's home through the connection", farHome + "/1", conn, true},
		{"the far node's plugin through the connection", farPl + "/4", conn, true},
		{"the connection does not serve this node's home", node + "/7", conn, false},
		{"this node's home serves its own grid", node + "/7", node, true},
		{"a bare unqualified id belongs to no chain", "7", fsPl, false},
		{"every source takes a bare id too", "7", EverySource, true},
		{"every source takes a chained id", farPl + "/4", EverySource, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ServedBy(tc.id, tc.source); got != tc.want {
				t.Errorf("ServedBy(%q, %q) = %v, want %v", tc.id, tc.source, got, tc.want)
			}
		})
	}
}
