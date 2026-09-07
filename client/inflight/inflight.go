// Package inflight owns two rules about the client's RPCs: every one is
// bounded, and a deduped fetch is deduped by key and never outlives the link
// it rode. Only the two long-lived streams, the event Subscribe and the shell
// WebSocket, are unbounded, because waiting is what they are for.
//
// The renderer fires a fetch on every cache miss, every frame, so a claim on
// the key keeps a second request from dogpiling the server. A claim that
// outlives its request would dedupe every later attempt away against a
// request that will never answer, leaving the pane loading with no error and
// no retry. So a claim ends in exactly two ways: the fetch returns, or
// CancelIf declares the link it rode gone. Deadline is the backstop for the
// reconnect that never comes, and done is scoped to the claim that made it,
// so a late release cannot free a fresher claim's key.
package inflight

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Deadline is the outside bound on any one client RPC. It is long enough for
// a plugin building its first listing over a slow link, and short enough that
// a request lost to a dead socket becomes a visible failure and a retry.
const Deadline = 30 * time.Second

// Bounded is the context every client RPC that holds no dedupe claim uses: a
// write, a nav walk's read, a probe, the boot handshake. It is the one door
// to Deadline for a caller with no Set of its own, so no call site decides
// the bound itself. The caller must cancel it.
//
// A write needs it because its answer acknowledges the outbox entry parked
// before it was sent, and a call that never returns would leave the user's
// bytes parked with no verdict and no drain.
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

// claim is one key's in-flight fetch. Identity is the pointer, so two claims
// on the same key over time are different claims and a late release is told
// from its successor's.
type claim struct{ cancel context.CancelFunc }

// New returns an empty Set whose fetches are bounded by d.
func New(d time.Duration) *Set {
	return &Set{d: d, m: map[string]*claim{}}
}

// Begin claims key for one fetch. ok is false when a fetch already holds the
// key, and the caller must not start a second one. The fetch must use the
// returned context, which carries the deadline and is what CancelIf cancels,
// and must call done when it returns.
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

// Context is a bounded context with no claim, for a fetch that is not
// deduped. CancelIf cannot reach it, so the caller must cancel it.
func (s *Set) Context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), s.d)
}

// release drops c's claim on key if c still holds it. A fetch CancelIf
// cancelled returns after a fresh fetch has taken the key, and freeing the
// fresh claim would drop the dogpile guard for as long as it runs.
func (s *Set) release(key string, c *claim) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[key] == c {
		delete(s.m, key)
	}
	c.cancel()
}

// CancelIf drops and cancels every claim whose key match reports, returning
// those keys sorted. Their link is gone, so they will never answer and their
// claims would keep every retry away. The caller chooses both which keys lost
// a link and which to ask for again, because one source going dark leaves
// every other source's fetches alive and still owed an answer.
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
