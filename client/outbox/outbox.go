// Package outbox is the ordered record of writes the server has not
// acknowledged, and the rule for what to do about them: local state may be
// dropped only on a server verdict. A write parks here as a retry thunk when
// it is sent, every completed attempt for the same key acks it away, and a
// transport failure leaves it parked. Send is that order and Record is that
// fork, each in one place, so no dispatcher can implement half of it. The
// retry kick and the unload flush drain what is parked.
//
// A write parks at send rather than on its answer, because a request the
// network eats never produces one, and the closure holding the user's bytes
// would die with its goroutine.
//
// An entry is order and retry, never a copy of the user's value. Every write
// that parks is a last-writer-wins overwrite of one key, so there is one live
// entry per key. A content write's thunk re-reads the bytes from the cache's
// content entry, which owns them, so the outbox knows which tiles owe the
// server a write while the bytes stay where the renderer reads them.
package outbox

import (
	"sync"

	"github.com/josephburnett/gridwell/client/clientsync"
)

// Key names one unacknowledged write: the dispatcher's label for the
// operation, such as "SetFraming", and the tile or grid id it targets.
type Key struct {
	Op string
	ID string
}

// OpContent is the label every user-content write parks under, so a tile's
// unsaved bytes have one entry however many paths tried to save them.
const OpContent = "Content"

// Outbox is the set of parked writes, drained in first-parked order.
type Outbox struct {
	mu    sync.Mutex
	m     map[Key]func()
	order []Key
}

// New returns an empty outbox.
func New() *Outbox {
	return &Outbox{m: map[Key]func(){}}
}

// Send is the order every non-content write runs in: park the retry thunk,
// run the call, then Record what the server said. It returns the outcome the
// call reported.
//
// The park comes first because a request that is never answered is never
// recorded either, and the value it carries has no other copy. The key stays
// parked while the call is out, which is the truth, so a drain racing the
// flight re-sends it, and that is safe because every parked write overwrites
// one key.
//
// retry may be nil for a write with nothing to park, such as a create or a
// drag whose ghost snaps back visibly. The call still runs and the outcome
// still acks any stale entry.
func (o *Outbox) Send(k Key, retry func(), call func() clientsync.Outcome) clientsync.Outcome {
	if retry != nil {
		o.Park(k, retry)
	}
	out := call()
	o.Record(out, k, retry)
	return out
}

// Record is the reconcile rule. A transport failure parks the write for the
// retry kick. Any other outcome acks the key, because the server spoke and
// the caller's own reaction resolves it from there. retry may be nil, and the
// outcome still acks any stale entry.
func (o *Outbox) Record(out clientsync.Outcome, k Key, retry func()) {
	if out == clientsync.OutcomeTransport && retry != nil {
		o.Park(k, retry)
		return
	}
	o.Ack(k)
}

// RecordContent syncs one tile's entry to the dirtiness of its bytes, which
// the client cache's content entry owns. It is Record's fork for the one op
// whose completion is a state rather than an RPC outcome, so a caller need
// not know whether the save landed, failed on transport, or was dropped by a
// verdict.
func (o *Outbox) RecordContent(tileID string, dirty bool, retry func()) {
	k := Key{Op: OpContent, ID: tileID}
	if dirty {
		o.Park(k, retry)
		return
	}
	o.Ack(k)
}

// SyncContent re-derives the content entries from the cache's dirty set
// before a drain. RecordContent already runs on every path that changes
// dirtiness, and this covers the drift, which would otherwise cost the words
// the user typed last at a quit, with no later sweep behind it.
//
// It parks what is dirty and acks nothing. It sees only the dirty ids, so a
// key it cannot see is a key it must not judge.
func (o *Outbox) SyncContent(dirty []string, retry func(tileID string) func()) {
	for _, id := range dirty {
		o.RecordContent(id, true, retry(id))
	}
}

// Park holds retry for k, replacing any earlier thunk for the same key,
// since the newer closure reaches the newer value. A replaced key keeps its
// original drain position.
func (o *Outbox) Park(k Key, retry func()) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.m[k]; !ok {
		o.order = append(o.order, k)
	}
	o.m[k] = retry
}

// Ack clears k, meaning an attempt for this key completed.
func (o *Outbox) Ack(k Key) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.m[k]; !ok {
		return
	}
	delete(o.m, k)
	o.order = compactOut(o.order, k)
}

// Drain removes every parked write and returns the retry thunks in
// first-parked order. A thunk re-parks itself through Record when the retry
// fails on transport again, so a drain during a dead link converges back to
// the same outbox instead of losing entries.
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

// Len reports how many writes are parked.
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
