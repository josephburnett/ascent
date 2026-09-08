// Package eventhub is the one event fan-out: a publisher never blocks on a
// slow subscriber and no distinct change is dropped. Each subscriber owns a
// coalescing queue keyed by the changed entity, so a newer event replaces the
// older undelivered one for that entity, matching the client cache's upsert by
// id, and the queue is bounded by the entities touched while a consumer
// stalls. An unkeyable event, key "", gets a unique key and never coalesces.
package eventhub

import (
	"strconv"
	"sync"
)

// streamBuffer is how many delivered events a stalled consumer can fall
// behind by before the pump blocks and later events coalesce in pending.
const streamBuffer = 16

type Hub[T any] struct {
	key  func(T) string
	mu   sync.Mutex
	subs map[*subscriber[T]]struct{}
}

// New's subscribers coalesce by key(ev).
func New[T any](key func(T) string) *Hub[T] {
	return &Hub[T]{key: key, subs: map[*subscriber[T]]struct{}{}}
}

type subscriber[T any] struct {
	mu      sync.Mutex
	keys    []string      // delivery order: first touch of each entity
	pending map[string]T  // latest event per entity key
	seq     int           // fallback key counter for unkeyable events
	wake    chan struct{} // pump signal, capacity 1
	done    chan struct{} // closed by cancel
	out     chan T        // consumer-facing stream, closed by the pump
}

// Subscribe's cancel detaches, and the pump then closes the stream.
func (h *Hub[T]) Subscribe() (<-chan T, func()) {
	sub := &subscriber[T]{
		pending: map[string]T{},
		wake:    make(chan struct{}, 1),
		done:    make(chan struct{}),
		out:     make(chan T, streamBuffer),
	}
	h.mu.Lock()
	h.subs[sub] = struct{}{}
	h.mu.Unlock()
	go sub.pump()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, sub)
			h.mu.Unlock()
			close(sub.done)
		})
	}
	return sub.out, cancel
}

// Publish never blocks.
func (h *Hub[T]) Publish(ev T) {
	key := h.key(ev)
	h.mu.Lock()
	subs := make([]*subscriber[T], 0, len(h.subs))
	for sub := range h.subs {
		subs = append(subs, sub)
	}
	h.mu.Unlock()
	for _, sub := range subs {
		sub.enqueue(key, ev)
	}
}

func (sub *subscriber[T]) enqueue(key string, ev T) {
	sub.mu.Lock()
	if key == "" {
		sub.seq++
		key = "u/" + strconv.Itoa(sub.seq)
	}
	if _, exists := sub.pending[key]; !exists {
		sub.keys = append(sub.keys, key)
	}
	sub.pending[key] = ev
	sub.mu.Unlock()
	select {
	case sub.wake <- struct{}{}:
	default:
	}
}

// pump delivers in first-touch order; events undelivered at cancel are dropped.
func (sub *subscriber[T]) pump() {
	defer close(sub.out)
	for {
		sub.mu.Lock()
		var ev T
		have := len(sub.keys) > 0
		if have {
			k := sub.keys[0]
			sub.keys = sub.keys[1:]
			ev = sub.pending[k]
			delete(sub.pending, k)
		}
		sub.mu.Unlock()
		if !have {
			select {
			case <-sub.wake:
				continue
			case <-sub.done:
				return
			}
		}
		select {
		case sub.out <- ev:
		case <-sub.done:
			return
		}
	}
}
