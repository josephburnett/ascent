// Package inflight owns two rules about the client's RPCs: every one of them
// is bounded, and a deduped fetch is deduped by key and never outlives the
// link it rode.
//
// Bounded is the first rule, and it covers reads and writes alike: a request
// the network swallows — a laptop asleep, a route that went away, a socket
// nobody answers and nobody resets — never returns on its own. Only the two
// long-lived streams (the event Subscribe and the shell WebSocket) are
// unbounded, because waiting is what they are for.
//
// The renderer fires a fetch on every cache miss, every frame, so a second
// request for a key the first has not answered yet would dogpile the server.
// The claim a fetch holds on its key is what prevents that — and the claim is
// also where a whole class of "it just sits there" bugs lived: a request that
// dies with the network without ever returning (a laptop sleep, a route that
// went away, a socket that is never answered and never reset) holds its key
// forever, so every later attempt is deduped away against a request that will
// never answer. The pane says "loading …" with no error and no retry.
//
// So a claim is bounded and cancellable, and there are exactly two ways it
// ends: the fetch returns (done), or the link it rode is declared gone
// (CancelIf — every claim on the event stream's reconnect resync, one
// source's on that source's health flap). Deadline is
// the backstop for the reconnect that never comes. A zombie's release can
// never take a fresher claim's key with it, because done is scoped to the
// claim that made it.
//
// Js-free and unit-tested; the wasm shim only wires it to the RPCs.
package inflight

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Deadline is the outside bound on any one client RPC, read or write:
// generous enough for a plugin building its first listing over a slow link,
// short enough that a request lost to a dead socket becomes a visible failure
// and a retry rather than a pane that waits forever. Any single call slower
// than this is already broken from where the user sits.
const Deadline = 30 * time.Second

// Bounded is the context every client RPC that holds no dedupe claim must
// use: a write, a nav walk's read, a probe, the boot handshake. It is the one
// door to Deadline for callers with no Set of their own, so "bounded" is not
// something each call site decides for itself. The caller must cancel it.
//
// A write reaching this matters twice over: its answer is what acknowledges
// the outbox entry parked before it was sent, so a call that never returns
// leaves the user's bytes parked forever with no verdict and no drain.
func Bounded() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), Deadline)
}

// Set is the live claims for one kind of fetch (grids, tiles, tile content),
// keyed by the id being fetched.
type Set struct {
	mu sync.Mutex
	d  time.Duration
	m  map[string]*claim
}

// claim is one key's in-flight fetch. Identity is the pointer: two claims on
// the same key over time are different claims, which is how a zombie's
// release is told from its successor's.
type claim struct{ cancel context.CancelFunc }

// New returns an empty Set whose fetches are bounded by d.
func New(d time.Duration) *Set {
	return &Set{d: d, m: map[string]*claim{}}
}

// Begin claims key for one fetch. ok is false when a fetch already holds the
// key and the caller must not start a second one. The returned context is the
// one the fetch must use: it carries the deadline and it is what CancelIf
// cancels. done releases the claim and must be called when the fetch returns.
func (s *Set) Begin(key string) (ctx context.Context, done func(), ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, held := s.m[key]; held {
		return nil, nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.d)
	c := &claim{cancel: cancel}
	s.m[key] = c
	return ctx, func() { s.release(key, c) }, true
}

// Context is a bounded context with no claim, for a fetch that is not deduped
// — the boot URL walk, which blocks on its own answer. It is bounded like
// every other fetch; it is simply not something CancelIf can reach, so the
// caller must cancel it.
func (s *Set) Context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), s.d)
}

// release drops c's claim on key, if c still holds it. A fetch cancelled by
// CancelIf returns late, after a fresh fetch has taken the key: the late
// release must not free the fresh claim, or the dogpile guard is gone for
// exactly as long as the new fetch runs.
func (s *Set) release(key string, c *claim) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[key] == c {
		delete(s.m, key)
	}
	c.cancel()
}

// CancelIf drops and cancels every claim whose key match reports, returning
// the keys it dropped in sorted order. The link those fetches rode is gone:
// they will never answer, and their claims would keep every retry away. The
// caller decides which of the returned keys to ask for again over the new
// link — and which fetches lost a link at all, because one source going dark
// leaves every other source's fetches alive and still owed an answer.
func (s *Set) CancelIf(match func(key string) bool) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.m))
	for k, c := range s.m {
		if !match(k) {
			continue
		}
		keys = append(keys, k)
		c.cancel()
		delete(s.m, k)
	}
	sort.Strings(keys)
	return keys
}

// Keys lists the keys with a fetch in flight, sorted.
func (s *Set) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.m))
	for k := range s.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Len is how many fetches are in flight.
func (s *Set) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}
