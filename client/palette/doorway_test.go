package palette

import (
	"testing"

	"github.com/josephburnett/gridwell/api/rpc"
	"github.com/josephburnett/gridwell/client/door"
)

// names is the section as the user reads it: one label per swatch, in order.
func names(sw []door.Place) []string {
	out := make([]string, 0, len(sw))
	for _, s := range sw {
		out = append(out, s.Plugin.Label)
	}
	return out
}

// The whole composition, as a table. One swatch per declared doorway: a row
// that names a grid of its own, plus every menu entry it declares. A plugin
// names no grid of its own, so it has no row on the menu — only its
// collections — and a plugin that declares nothing shows nothing.
func TestDoorwaysTable(t *testing.T) {
	entry := func(id, label, grid string) rpc.MenuEntry {
		return rpc.MenuEntry{ID: id, Label: label, GridID: grid}
	}
	cases := []struct {
		name string
		rows []rpc.PluginInfo
		want []string
	}{{
		name: "a node's home is a place: its own swatch, then its entries",
		rows: []rpc.PluginInfo{{UUID: "n1", Label: "home", RootGridID: "n1/1",
			MenuEntries: []rpc.MenuEntry{entry("trash", "trash", "n1/9")}}},
		want: []string{"home", "home · trash"},
	}, {
		name: "a plugin with three collections is three swatches and no row",
		rows: []rpc.PluginInfo{{UUID: "hey", Kind: "mail", Label: "hey", MenuEntries: []rpc.MenuEntry{
			entry("imbox", "Imbox", "hey/1"),
			entry("feed", "Feed", "hey/2"),
			entry("paper_trail", "Paper Trail", "hey/3"),
		}}},
		want: []string{"hey · Imbox", "hey · Feed", "hey · Paper Trail"},
	}, {
		name: "a single-collection plugin declares no entry label and reads as itself",
		rows: []rpc.PluginInfo{{UUID: "fs", Kind: "fs", Label: "files",
			MenuEntries: []rpc.MenuEntry{entry(".", "", "fs/1")}}},
		want: []string{"files"},
	}, {
		name: "a plugin that declares nothing contributes nothing",
		rows: []rpc.PluginInfo{{UUID: "fs", Kind: "fs", Label: "files"}},
		want: []string{},
	}, {
		name: "a failure is still a swatch, so it can be seen and asked about",
		rows: []rpc.PluginInfo{{UUID: "fs", Kind: "fs", Label: "files",
			InfoError: "plugin not responding: boom"}},
		want: []string{"files"},
	}, {
		name: "a connection is a place, answered or not",
		rows: []rpc.PluginInfo{
			rpc.ConnectionRow(rpc.ConnectionInfo{UUID: "n1/rtb", Label: "rtb", RootGridID: "n1/rtb/1"}),
			rpc.ConnectionRow(rpc.ConnectionInfo{UUID: "n1/far", Label: "far"}),
		},
		want: []string{"rtb", "far"},
	}, {
		name: "an entry with no grid is not a doorway",
		rows: []rpc.PluginInfo{{UUID: "hey", Label: "hey", MenuEntries: []rpc.MenuEntry{
			entry("imbox", "Imbox", "hey/1"), entry("feed", "Feed", ""),
		}}},
		want: []string{"hey · Imbox"},
	}, {
		name: "rows keep handshake order, each row's entries directly after it",
		rows: []rpc.PluginInfo{
			{UUID: "n1", Label: "home", RootGridID: "n1/1",
				MenuEntries: []rpc.MenuEntry{entry("trash", "trash", "n1/9")}},
			{UUID: "hey", Label: "hey", MenuEntries: []rpc.MenuEntry{entry("feed", "Feed", "hey/2")}},
			rpc.ConnectionRow(rpc.ConnectionInfo{UUID: "n1/rtb", Label: "rtb", RootGridID: "n1/rtb/1"}),
		},
		want: []string{"home", "home · trash", "hey · Feed", "rtb"},
	}}
	for _, c := range cases {
		got := names(Doorways(c.rows))
		if len(got) != len(c.want) {
			t.Errorf("%s: swatches = %q, want %q", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: swatches = %q, want %q", c.name, got, c.want)
				break
			}
		}
	}
}

// Every swatch is enterable: it names a doorway, so a click has somewhere to
// go. That is what makes "one swatch per declared doorway" the whole rule —
// except a row the menu shows in order to report on it, which is exactly the
// row that is not healthy.
func TestEverySwatchNamesADoorwayOrAFailure(t *testing.T) {
	rows := []rpc.PluginInfo{
		{UUID: "n1", Label: "home", RootGridID: "n1/1",
			MenuEntries: []rpc.MenuEntry{{ID: "trash", Label: "trash", GridID: "n1/9"}}},
		{UUID: "hey", Label: "hey", MenuEntries: []rpc.MenuEntry{{ID: "feed", Label: "Feed", GridID: "hey/2"}}},
	}
	for _, s := range Doorways(rows) {
		if s.Plugin.RootGridID == "" {
			t.Errorf("swatch %q names no grid to descend into", s.Plugin.Label)
		}
	}
}

// The pseudo-row of an entry carries no entries of its own: they belong to
// the declaring row, and a second pass over them would show every collection
// twice.
func TestEntrySwatchDoesNotCarryTheRowsEntries(t *testing.T) {
	rows := []rpc.PluginInfo{{UUID: "hey", Label: "hey", MenuEntries: []rpc.MenuEntry{
		{ID: "imbox", Label: "Imbox", GridID: "hey/1"},
		{ID: "feed", Label: "Feed", GridID: "hey/2"},
	}}}
	sw := Doorways(rows)
	if len(sw) != 2 {
		t.Fatalf("swatches = %q, want the two collections", names(sw))
	}
	for _, s := range sw {
		if len(s.Plugin.MenuEntries) != 0 {
			t.Errorf("%q carries %d entries of its own", s.Plugin.Label, len(s.Plugin.MenuEntries))
		}
		if s.Entry == nil {
			t.Errorf("%q must name the entry it came from", s.Plugin.Label)
		}
	}
}
