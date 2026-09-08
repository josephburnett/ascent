package store

import (
	gridwellv1 "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
)

// SubscribeEvents registers a subscriber and returns its event stream. Call
// the returned cancel func to detach; the stream is closed by the pump.
func (s *Store) SubscribeEvents() (<-chan *gridwellv1.Event, func()) {
	return s.hub.Subscribe()
}

// publish hands the event to every subscriber's queue. Never blocks.
func (s *Store) publish(ev *gridwellv1.Event) {
	s.hub.Publish(ev)
}
