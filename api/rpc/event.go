package rpc

import (
	pb "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
)

// EventKey names the entity a wire event is about, so internal/eventhub can
// replace an older undelivered event for the same entity and drop no distinct
// one. "" is unkeyable and never coalesces. It is one arm set for every hub:
// a publisher that emits no health event is unaffected by the health arm,
// while two hubs disagreeing would coalesce the same wire event two ways.
func EventKey(ev *pb.Event) string {
	switch p := ev.GetPayload().(type) {
	case *pb.Event_GridChanged:
		return "g/" + p.GridChanged.GetGridId()
	case *pb.Event_TileChanged:
		return "t/" + p.TileChanged.GetTile().GetId()
	case *pb.Event_TileRemoved:
		// Keyed apart from TileChanged, and by grid: a cross-grid move emits
		// TileRemoved for the source then TileChanged for the destination,
		// for the same tile id, and both must reach the consumer.
		return "r/" + p.TileRemoved.GetGridId() + "/" + p.TileRemoved.GetTileId()
	case *pb.Event_PluginHealth:
		return "h/" + p.PluginHealth.GetPluginUuid()
	}
	return ""
}
