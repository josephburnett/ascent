package store

import (
	gridwellv1 "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
)

// The store's event fan-out is internal/eventhub. This file owns only the key,
// which entity an event is about, so a newer event for the same entity can
// replace an older undelivered one.

// SubscribeEvents registers a subscriber and returns its event stream. Call
// the returned cancel func to detach; the stream is closed by the pump.
func (s *Store) SubscribeEvents() (<-chan *gridwellv1.Event, func()) {
	return s.hub.Subscribe()
}

// publish hands the event to every subscriber's queue. Never blocks.
func (s *Store) publish(ev *gridwellv1.Event) {
	s.hub.Publish(ev)
}

// eventKey identifies the entity an event is about. "" means unkeyable, and
// enqueue gives those a unique key so they are never coalesced.
func eventKey(ev *gridwellv1.Event) string {
	switch p := ev.Payload.(type) {
	case *gridwellv1.Event_GridChanged:
		return "g/" + p.GridChanged.GetGridId()
	case *gridwellv1.Event_TileChanged:
		return "t/" + p.TileChanged.GetTile().GetId()
	case *gridwellv1.Event_TileRemoved:
		// Keyed apart from TileChanged, and by grid: a cross-grid move emits
		// TileRemoved for the source then TileChanged for the destination,
		// for the same tile id, and both must reach the consumer.
		return "r/" + p.TileRemoved.GetGridId() + "/" + p.TileRemoved.GetTileId()
	}
	return ""
}
