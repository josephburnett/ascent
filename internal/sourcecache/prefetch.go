package sourcecache

// Whole-source prefetch. The cache remembers what you touched; this walker
// warms what you did not, so "everything on this source is readable offline"
// is literally true rather than everything-visited. It is a per-namespace
// policy, Options.Prefetch, and not part of the engine: the transport opts in
// because a connection's absence is a machine going dark, while a local
// plugin's source is right here and never crawls. The data is small by
// construction — text, previews, metadata — so the walk is a full traversal
// with caps as emergency valves, not a sampling strategy.
//
// There are two triggers, and kickPrefetch is where both are written down.
// One is every Layer.Subscribe establishment, which warms every source the
// namespace declares: in front of the transport that stream is the connection
// hub's, one for every connection, so it means the initial connect and a
// re-dial of the server's own fan-in. The other is a source coming back —
// Layer.setDark's transition out of darkness, from a relayed health-up or
// from a pass-through call answering again, whichever notices first — which
// warms that ONE source, because the hub survives a single connection's
// outage and nothing else here would notice it returned. A recovery is when
// the promise is most stale: what the user re-opens is refetched by the
// client's blunt kick, and what nobody re-opened is exactly what this walk is
// for. The walk runs through the wrapper's own read methods, so every answer
// lands in the cache by the one existing write path and there is no second
// writer; it reads only, claims no version, and going dark does nothing.
//
// A transport failure mid-walk aborts quietly: the source went dark, and the
// next trigger — a Subscribe, or that source coming back — walks again. A
// whole-source walk aborts on the first dark source it meets, and the
// recovery trigger is what picks that source up again. A coded refusal on an
// individual read — a tombstoned segment, a permission wall — skips that
// branch and keeps walking, because the walker must never invent
// reachability the source denies. A serves_page tile's door body, at its root
// subpath, is walked under the same byte budget, so photos and plugin pages
// are offline too in the common case.

import (
	"context"
	"github.com/josephburnett/gridwell/api/gwerr"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	pb "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
)

// Emergency valves, not tuning knobs: a source that trips one is far outside
// the small-data model this cache is built for, and the walk simply stops
// warming there. Everything already walked stays cached.
var (
	// prefetchMaxGrids caps the traversal breadth.
	prefetchMaxGrids = 4096
	// prefetchContentBudget caps the sum of content bodies fetched by one
	// walk; per-entry bodies are already capped by maxCachedContentBytes.
	prefetchContentBudget = 256 << 20
	// prefetchPause is the politeness gap between RPCs so a background
	// walk never crowds out the user's own reads on a slow link.
	prefetchPause = 2 * time.Millisecond
)

// contentKinds are the tile kinds whose bodies the walk fetches: the ones
// whose content is what the user came for offline, such as a note's text or a
// pane tile's layout. Everything else renders offline from its cached row and
// preview.
var contentKinds = map[string]bool{"text": true, "pane": true}

// prefetcher is the walk's single-flight state, one per Client. running is
// the walks in flight keyed by what each covers: "" is every source, a
// connection segment is that one.
type prefetcher struct {
	mu      sync.Mutex
	running map[string]bool
	// closed is set before the closer waits, under the same mutex a kick
	// takes, so no walk can be started once waiting has begun. A trigger now
	// rides every pass-through call, and one landing during Close would
	// otherwise race the WaitGroup it is counted by.
	closed bool
	ctx    context.Context
	cancel context.CancelFunc
	// wg counts the running walk. The closer waits on it after cancelling and
	// before closing the DB, so a walk never writes into a closed cache.
	wg sync.WaitGroup
}

// kickPrefetch is the one owner of when a walk starts, and it starts on two
// triggers, both landing here: a Layer.Subscribe establishment, with source
// "" for every source the namespace declares, and a source coming back
// (Layer.setDark), with that source's segment for that source alone. It runs
// only where the walk is this namespace's policy (Options.Prefetch).
//
// It is serialized per source and never queued: a trigger for ground a
// running walk already covers is satisfied by that walk's own freshness,
// since a trigger means the source is up now and the running walk is already
// exploiting that. That is also the flap guard — a connection that leaves and
// returns twenty times has at most one walk in flight, never twenty stacked.
func (c *Layer) kickPrefetch(source string) {
	if !c.opts.Prefetch {
		return
	}
	c.pf.mu.Lock()
	// running[""] is the whole-source walk, which covers every source: a
	// recovery arriving under one is that walk's to warm.
	if c.pf.ctx == nil || c.pf.closed || c.pf.running[""] || c.pf.running[source] {
		c.pf.mu.Unlock()
		return
	}
	c.pf.running[source] = true
	c.pf.wg.Add(1)
	ctx := c.pf.ctx
	c.pf.mu.Unlock()
	go func() {
		defer c.pf.wg.Done()
		defer func() {
			c.pf.mu.Lock()
			delete(c.pf.running, source)
			c.pf.mu.Unlock()
		}()
		c.prefetch(ctx, source)
	}()
}

// stopWalks refuses further kicks and waits for the walks in flight, so a
// walk never writes into a closed cache. Refusing first is the point: a
// trigger rides every pass-through call now, and one landing between the
// cancel and the wait would be counted by a WaitGroup already being waited
// on.
func (c *Layer) stopWalks() {
	c.pf.mu.Lock()
	c.pf.closed = true
	c.pf.mu.Unlock()
	c.pf.cancel()
	c.pf.wg.Wait()
}

// prefetch is the walk itself, over one source or over all of them: it goes
// through the wrapper's own read methods, warming grids, tiles, previews,
// plugin lists, and content bodies. Source "" warms every root the namespace
// declares and a connection segment warms only the roots that belong to it —
// one walk implementation, taken from one end or from one branch of it.
// kickPrefetch runs it in the background; a test runs it synchronously.
func (c *Layer) prefetch(ctx context.Context, source string) {
	w := &walker{c: c, ctx: ctx, seenGrids: map[string]bool{}, seenTiles: map[string]bool{}, seenNs: map[string]bool{}}
	// The roots are what the fronted namespace declares about itself, through
	// the door every namespace answers on: the handshake with no namespace.
	// The transport's answer is its connections, each with the landing it
	// learned; a node-shaped one names its plugins' roots. Info was the old
	// door and the transport implements none, so the walk returned at its
	// first line and the whole-source warm silently did nothing.
	hs, err := c.Handshake(ctx, &pb.HandshakeRequest{})
	if err != nil {
		return // dark, or refused, at the doorstep: nothing to walk
	}
	roots := []string{}
	add := func(id string) {
		// A root belongs to the source its id names, which is how one
		// source's walk is a filter over the same declaration rather than a
		// second way of asking. A source key deeper than a connection segment
		// — a far plugin's health — owns no root here and warms nothing: a
		// plugin being down never made the machine unreachable.
		if id == "" || (source != "" && sourceOf(id) != source) {
			return
		}
		roots = append(roots, id)
	}
	for _, conn := range hs.GetConnections() {
		add(conn.GetRootGridId())
	}
	for _, p := range hs.GetPlugins() {
		add(p.GetRootGridId())
	}
	for _, g := range roots {
		if !w.walkGrid(g) {
			return
		}
	}
}

type walker struct {
	c         *Layer
	ctx       context.Context
	seenGrids map[string]bool
	seenTiles map[string]bool
	seenNs    map[string]bool
	spent     int
}

// pause is the politeness gap. false means the walk should stop, because the
// context is done and the client is closing.
func (w *walker) pause() bool {
	select {
	case <-w.ctx.Done():
		return false
	case <-time.After(prefetchPause):
		return true
	}
}

// walkGrid warms one grid and recurses into its children. It returns false
// only when the walk should abort — the source is dark, the context is done,
// or a cap tripped. A coded refusal skips the branch and returns true.
func (w *walker) walkGrid(gridID string) bool {
	if w.seenGrids[gridID] || len(w.seenGrids) >= prefetchMaxGrids {
		return len(w.seenGrids) < prefetchMaxGrids
	}
	w.seenGrids[gridID] = true
	if !w.pause() {
		return false
	}
	// The live read, never the serve-first door: the walk exists to warm the
	// cache from the source, and a remembered answer would warm nothing.
	resp, err := w.c.getGridLive(w.ctx, gridID)
	if err != nil {
		return !gwerr.IsTransport(err) // dark aborts; a refusal skips this branch
	}
	// The + menu context for this grid's node: warm the routed plugin list
	// once per namespace.
	if ns := resp.GetGrid().GetNodeNs(); !w.seenNs[ns] {
		w.seenNs[ns] = true
		if w.pause() {
			if _, err := w.c.Handshake(w.ctx, &pb.HandshakeRequest{Namespace: ns}); err != nil && gwerr.IsTransport(err) {
				return false
			}
		} else {
			return false
		}
	}
	for _, t := range resp.GetTiles() {
		if !w.walkTile(t) {
			return false
		}
	}
	for _, t := range resp.GetTiles() {
		if child := t.GetChildGridId(); child != "" {
			if !w.walkGrid(child) {
				return false
			}
		}
	}
	return true
}

// walkTile warms one tile's preview and, for a content kind, its body,
// following a leaf link to its target row once.
func (w *walker) walkTile(t *pb.Tile) bool {
	if w.seenTiles[t.GetId()] {
		return true
	}
	w.seenTiles[t.GetId()] = true
	if !w.pause() {
		return false
	}
	if _, err := w.c.GetTilePreview(w.ctx, &pb.GetTilePreviewRequest{TileId: t.GetId()}); err != nil && gwerr.IsTransport(err) {
		return false
	}
	if contentKinds[t.GetKind()] && w.spent < prefetchContentBudget {
		if !w.pause() {
			return false
		}
		n, ok := drain(func(count func(int)) error {
			return w.c.ReadContent(w.ctx, &pb.ReadContentRequest{TileId: t.GetId()},
				func(ch *pb.ContentChunk) error { count(len(ch.GetData())); return nil })
		})
		if !ok {
			return false
		}
		w.spent += n
	}
	// A serves_page tile's face-value body is its door page at the root
	// subpath, which is rpc.PageURL's target. It is bounded like every body:
	// the entry cap skips an oversized page and the budget stops the class.
	if t.GetServesPage() && w.spent < prefetchContentBudget {
		if !w.pause() {
			return false
		}
		n, ok := drain(func(count func(int)) error {
			return w.c.ServeContent(w.ctx, &pb.ServeContentRequest{TileId: t.GetId(), Subpath: ""},
				func(ch *pb.ServeContentChunk) error { count(len(ch.GetData())); return nil })
		})
		if !ok {
			return false
		}
		w.spent += n
	}
	// A leaf link's target is what the link renders and resolves through, so
	// warm the target row, and its face and body, even if its own grid is
	// never walked.
	//
	// walkTile owns the seen-set: pre-marking the target here would make the
	// recursion below return at its seen-check with nothing warmed, and would
	// poison the target's later natural visit too.
	if target := t.GetLinkTargetId(); target != "" && !w.seenTiles[target] {
		if !w.pause() {
			return false
		}
		tr, err := w.c.GetTile(w.ctx, &pb.GetTileRequest{TileId: target})
		if err != nil {
			return !gwerr.IsTransport(err)
		}
		if tt := tr.GetTile(); tt != nil {
			tt2 := proto.Clone(tt).(*pb.Tile)
			tt2.LinkTargetId = "" // never chase a chain twice
			return w.walkTile(tt2)
		}
	}
	return true
}

// drain runs one content read purely to warm the cache — the tee behind it
// stores what passes — and reports the bytes seen. ok=false means the source
// went dark before a single chunk, so the walk aborts; a failure after bytes
// have flowed, or a coded refusal, just ends this branch.
func drain(read func(count func(int)) error) (n int, ok bool) {
	first := true
	err := read(func(size int) {
		first = false
		n += size
	})
	if err != nil {
		return n, !(first && gwerr.IsTransport(err))
	}
	return n, true
}
