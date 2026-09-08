// Package outbox is the ordered record of writes the server has not
// acknowledged: local state may be dropped only on a server verdict. A write
// parks as a retry thunk when it is sent, not on its answer, because a request
// the network eats never produces one. Send is that order and Record is the
// fork, each in one place, so no dispatcher implements half of it.
//
// An entry is order and retry, never a copy of the user's value. Every parked
// write is a last-writer-wins overwrite of one key, so there is one entry per
// key, and a content thunk re-reads the bytes from the cache entry that owns
// them.
package outbox

import (
	"sync"

	"github.com/josephburnett/gridwell/client/clientsync"
)

// Key names one unacknowledged write: the dispatcher's label, such as
// "SetFraming", and the tile or grid id it targets.
type Key struct {
	Op string
	ID string
}

// OpContent is the label every user-content write parks under, so a tile's
// unsaved bytes have one entry however many paths tried to save them.
const OpContent = "Content"

// Outbox drains in first-parked order.
type Outbox struct {
	mu    sync.Mutex
	m     map[Key]func()
	order []Key
}

func New() *Outbox {
	return &Outbox{m: map[Key]func(){}}
}

// Send is the order every non-content write runs in: park the retry thunk,
// run the call, then Record what the server said. The key stays parked while
// the call is out, so a drain racing the flight re-sends it, which is safe
// because every parked write overwrites one key. retry may be nil for a write
// with nothing to park, such as a drag whose ghost snaps back visibly.
func (o *Outbox) Send(k Key, retry func(), call func() clientsync.Outcome) clientsync.Outcome {
	if retry != nil {
		o.Park(k, retry)
	}
	out := call()
	o.Record(out, k, retry)
	return out
}

// Record is the reconcile rule. A transport failure parks the write for the
// retry kick; any other outcome acks the key, because the server spoke and
// the caller's own reaction resolves it from there.
func (o *Outbox) Record(out clientsync.Outcome, k Key, retry func()) {
	if out == clientsync.OutcomeTransport && retry != nil {
		o.Park(k, retry)
		return
	}
	o.Ack(k)
}

// RecordContent syncs one tile's entry to the dirtiness the cache's content
// entry owns. It is Record's fork for the one op whose completion is a state
// rather than an RPC outcome.
func (o *Outbox) RecordContent(tileID string, dirty bool, retry func()) {
	k := Key{Op: OpContent, ID: tileID}
	if dirty {
		o.Park(k, retry)
		return
	}
	o.Ack(k)
}

// SyncContent re-derives the content entries from the cache's dirty set
// before a drain, covering drift that would otherwise cost the words typed
// last at a quit. It parks what is dirty and acks nothing: a key it cannot
// see is a key it must not judge.
func (o *Outbox) SyncContent(dirty []string, retry func(tileID string) func()) {
	for _, id := range dirty {
		o.RecordContent(id, true, retry(id))
	}
}

// Park replaces any earlier thunk for k, since the newer closure reaches the
// newer value, and keeps the key's original drain position.
func (o *Outbox) Park(k Key, retry func()) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.m[k]; !ok {
		o.order = append(o.order, k)
	}
	o.m[k] = retry
}

// Ack clears k: an attempt for this key completed.
func (o *Outbox) Ack(k Key) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.m[k]; !ok {
		return
	}
	delete(o.m, k)
	o.order = compactOut(o.order, k)
}

// Drain removes every parked write and returns the thunks in first-parked
// order. A thunk re-parks itself through Record when its retry fails on
// transport again, so a drain during a dead link loses no entries.
func (o *Outbox) Drain() []func() {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]func(), 0, len(o.m))
	for _, k := range o.order {
		if fn, ok := o.m[k]; ok {
			out = append(out, fn)
		}
	}
	o.m = map[Key]func(){}
	o.order = nil
	return out
}

func (o *Outbox) Len() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.m)
}

// Keys returns the parked keys in drain order, for reading only.
func (o *Outbox) Keys() []Key {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]Key, 0, len(o.m))
	for _, k := range o.order {
		if _, ok := o.m[k]; ok {
			out = append(out, k)
		}
	}
	return out
}

func compactOut(order []Key, k Key) []Key {
	out := order[:0]
	for _, o := range order {
		if o != k {
			out = append(out, o)
		}
	}
	return out
}
