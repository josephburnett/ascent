package rpc

import (
	"reflect"
	"testing"

	pb "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
)

// rpc.Tile, rpc.Grid and their conversions are generated from the proto
// into wire_gen.go, and wire_test.go pins the wire's exhaustive round trip
// and JSON shape. What this file covers is the hand-written shapes below,
// which the generator deliberately does not produce.

// TestTilesSliceProto: nil slices stay nil (not []), and a populated slice
// round-trips element-for-element.
func TestTilesSliceProto(t *testing.T) {
	if TilesToProto(nil) != nil {
		t.Error("TilesToProto(nil) should be nil")
	}
	if TilesFromProto(nil) != nil {
		t.Error("TilesFromProto(nil) should be nil")
	}
	in := []Tile{*exhaustiveTile(t), {ID: "x", Kind: KindText, X: 1}}
	got := TilesFromProto(TilesToProto(in))
	if !reflect.DeepEqual(in, got) {
		t.Errorf("tiles slice round-trip diverged:\n in = %+v\nout = %+v", in, got)
	}
}

// TestEventFromProto covers all four event kinds: the wire oneof arm must
// decode to the matching discriminator and the one populated payload.
func TestEventFromProto(t *testing.T) {
	cases := []struct {
		wire *pb.Event
		want Event
	}{
		{&pb.Event{Payload: &pb.Event_GridChanged{GridChanged: &pb.GridChanged{GridId: "g-1"}}},
			Event{Kind: EventGridChanged, GridChanged: &GridChanged{GridID: "g-1"}}},
		{&pb.Event{Payload: &pb.Event_TileChanged{TileChanged: &pb.TileChanged{Tile: TileToProto(exhaustiveTile(t))}}},
			Event{Kind: EventTileChanged, TileChanged: &TileChanged{Tile: *exhaustiveTile(t)}}},
		{&pb.Event{Payload: &pb.Event_TileRemoved{TileRemoved: &pb.TileRemoved{GridId: "g-2", TileId: "t-3"}}},
			Event{Kind: EventTileRemoved, TileRemoved: &TileRemoved{GridID: "g-2", TileID: "t-3"}}},
		{&pb.Event{Payload: &pb.Event_PluginHealth{PluginHealth: &pb.EventPluginHealth{PluginUuid: "u-1", Detail: "dial tcp: connection refused"}}},
			Event{Kind: EventPluginHealth, PluginHealth: &PluginHealth{PluginUUID: "u-1", Detail: "dial tcp: connection refused"}}},
		{&pb.Event{Payload: &pb.Event_PluginHealth{PluginHealth: &pb.EventPluginHealth{PluginUuid: "u-1", Healthy: true}}},
			Event{Kind: EventPluginHealth, PluginHealth: &PluginHealth{PluginUUID: "u-1", Healthy: true}}},
	}
	for _, c := range cases {
		if got := EventFromProto(c.wire); !reflect.DeepEqual(c.want, got) {
			t.Errorf("event decode (%v) diverged:\n want = %+v\n got  = %+v", c.want.Kind, c.want, got)
		}
	}
}

// TestEventFromProtoNil: a nil or empty wire event decodes to the zero Event
// rather than panicking on a nil payload.
func TestEventFromProtoNil(t *testing.T) {
	if got := EventFromProto(nil); got.Kind != "" {
		t.Errorf("EventFromProto(nil) = %+v, want zero", got)
	}
	if got := EventFromProto(&pb.Event{}); got.Kind != "" {
		t.Errorf("EventFromProto(empty) = %+v, want zero", got)
	}
}

// TestCreateConvertersSelectKindAndFields: each typed create maps onto the
// unified CreateTile shape with the right Kind and the kind-specific fields
// in the right place. This mapping lives in one spot per primitive.
func TestCreateConvertersSelectKindAndFields(t *testing.T) {
	well := CreateWellToProto(&CreateWellRequest{GridID: "g", X: 1, Y: 2, W: 3, H: 4, ChildGridID: "cg", Label: "home",
		Framing: Framing{Cx: 7, Cy: 8, Zoom: 0.5}})
	if well.Tile.Kind != KindWell || well.Tile.ChildGridId != "cg" || well.Tile.AltText != "home" || well.GridId != "g" {
		t.Errorf("CreateWell mapping wrong: %+v", well.Tile)
	}
	// The framing seed rides the create so an exit well, such as a plugin
	// link dropped from the + menu, starts at the target's persisted root
	// view.
	if well.Tile.ViewCx != 7 || well.Tile.ViewCy != 8 || well.Tile.ViewZoom != 0.5 {
		t.Errorf("CreateWell dropped the view framing seed: %+v", well.Tile)
	}
	text := CreateTextToProto(&CreateTextRequest{GridID: "g", W: 2, H: 2})
	if text.Tile.Kind != KindText || text.Tile.W != 2 {
		t.Errorf("CreateText mapping wrong: tile=%+v", text.Tile)
	}
	u := CreateURLToProto(&CreateURLRequest{GridID: "g", URL: "https://x"})
	if u.Tile.Kind != KindURL || u.Tile.UrlString != "https://x" {
		t.Errorf("CreateURL mapping wrong: %+v", u.Tile)
	}
	sh := CreateShellToProto(&CreateShellRequest{GridID: "g", W: 2, H: 2})
	if sh.Tile.Kind != KindShell {
		t.Errorf("CreateShell mapping wrong: %+v", sh.Tile)
	}
}

// TestSetConvertersAreFramingByKind: the SetTile converters carry framing
// and preview fields under the right Kind, and route the JPEG preview
// through the Preview field rather than the tile body. This is the
// framing-versus-content boundary.
func TestSetConvertersAreFramingByKind(t *testing.T) {
	// Grid framing is not here: it rides its own verb, SetFraming, the one
	// door for both rows that can own it.
	f := SetFramingToProto(&SetFramingRequest{TileID: "t", Framing: Framing{Cx: 4, Cy: 5, Zoom: 6}})
	if f.TileId != "t" || f.Cx != 4 || f.Cy != 5 || f.Zoom != 6 {
		t.Errorf("SetFraming mapping wrong: %+v", f)
	}
	txt := SetTextViewToProto(&SetTextViewRequest{TileID: "t", TextX: 1, TextY: 2, TextW: 3, TextH: 4, TextMode: "rendered"})
	if txt.Tile.Kind != KindText || txt.Tile.TextMode != "rendered" || txt.Tile.TextW != 3 {
		t.Errorf("SetTextView mapping wrong: %+v", txt.Tile)
	}
	urlState := SetURLStateToProto(&SetURLStateRequest{TileID: "t", URL: "https://x", Title: "T", JPEG: []byte("j")})
	if urlState.Tile.Kind != KindURL || urlState.Tile.UrlString != "https://x" || urlState.Tile.AltText != "T" || string(urlState.Preview) != "j" {
		t.Errorf("SetURLState mapping wrong: tile=%+v preview=%q", urlState.Tile, urlState.Preview)
	}
	shellPrev := SetShellPreviewToProto(&SetShellPreviewRequest{TileID: "t", JPEG: []byte("j")})
	if shellPrev.Tile.Kind != KindShell || string(shellPrev.Preview) != "j" {
		t.Errorf("SetShellPreview mapping wrong: tile=%+v preview=%q", shellPrev.Tile, shellPrev.Preview)
	}
}
