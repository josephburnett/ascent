//go:build connections

// The mid-session partition gate: a mount that dies under a live session. It
// runs the production binaries through a real ssh tunnel, reads through the
// mount to warm the node-side cache in internal/sourcecache, then SIGKILLs the
// remote node and asserts the whole offline story end to end: a warmed read
// serves stale, never-read bytes fail honestly, the offline deep copy degrades
// as decided — cached copies, uncached links — and a revived remote answers
// live again, so the cache never masks a healed mount. The revival then warms
// the source again (#275): bytes nobody ever read survive a SECOND partition,
// because the recovery re-kicked the prefetch walk.

package connections_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gwrpc "github.com/josephburnett/gridwell/api/rpc"
	"github.com/josephburnett/gridwell/internal/connection/dial/dialtest"
)

// awaitConnHealth reads the client's event stream until one connection's
// health says what is expected. It is the real path a connection's health
// takes: fan-in → qualification → the client's stream, through the source
// cache's own arm on the way.
func awaitConnHealth(t *testing.T, health <-chan gwrpc.Event, conn string, want bool) {
	t.Helper()
	deadline := time.After(90 * time.Second)
	for {
		select {
		case ev := <-health:
			if h := ev.PluginHealth; h != nil && strings.Contains(h.PluginUUID, conn) && h.Healthy == want {
				return
			}
		case <-deadline:
			t.Fatalf("connection %s never reported healthy=%v", conn, want)
		}
	}
}

func TestMountPartitionServesCache(t *testing.T) {
	root := repoRoot(t)
	bin := filepath.Join(root, "gridwell")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("gridwell binary not built (run `make build`): %v", err)
	}
	ctx := context.Background()
	num := func(v any) int64 { f, _ := v.(float64); return int64(f) }

	// Remote node: one local. Keep its address — the revival must land on
	// the SAME addr the connection dials.
	remoteHome := t.TempDir()
	freshHome(t, remoteHome)
	// The connection door's socket lives under the home, so a revival on the same
	// home lands on the SAME path the connection dials.
	remoteOrigin, remoteAddr, stopRemote := startServeProc(t, bin, remoteHome, "127.0.0.1:0")
	creds := dialtest.Server(t, t.TempDir())

	// Local node: localdb + the builtin transport; the connection is
	// server.yaml CONFIG (v2 #269), declared before first serve.
	localHome := t.TempDir()
	freshHome(t, localHome)
	appendConnectionsYAML(t, localHome, sshConnectionYAML(t, "partconn1", creds, remoteAddr))
	localOrigin, _ := startServe(t, bin, localHome, "127.0.0.1:0")
	cl := clientFor(localOrigin)

	// The connection lands on the remote HOME — personal's root grid
	// (remote-menu, 2026-08-16) — writable directly. The transport's id
	// (the cache file's name) is the row's leading segment.
	personalChild := awaitConnRoot(t, localOrigin, "partconn1")
	lp := rpc(t, localOrigin, "Handshake", map[string]any{})
	var homeRoot string
	for _, p := range lp["plugins"].([]any) {
		pm := p.(map[string]any)
		if pm["label"] == "home" {
			homeRoot, _ = pm["rootGridId"].(string)
		}
	}

	// One live subscription, the way the real client holds one: it is what
	// carries a connection's health down through the source cache, and the
	// establishment it makes is the whole-source walk's own trigger. Opened
	// BEFORE the tiles exist, and given a moment to settle, so nothing below
	// can have been warmed by that walk. The whole stream lives in its own
	// goroutine: over HTTP/1.1 the call does not return until the first event
	// arrives, while the server-side subscription — and its walk — starts as
	// soon as the request lands.
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()
	health := make(chan gwrpc.Event, 64)
	go func() {
		stream, serr := cl.Subscribe(subCtx)
		if serr != nil {
			return
		}
		defer stream.Close()
		for {
			ev, ok, rerr := stream.Recv()
			if !ok || rerr != nil {
				return
			}
			if ev.PluginHealth != nil {
				select {
				case health <- ev:
				default:
				}
			}
		}
	}()
	time.Sleep(3 * time.Second)

	// Through the chain: a well holding a WARMED text, a NEVER-READ text.
	well := rpc(t, localOrigin, "CreateTile", map[string]any{
		"gridId": personalChild,
		"tile":   map[string]any{"kind": "well", "x": 0, "y": 0, "w": 1, "h": 1, "altText": "trip"},
	})["tile"].(map[string]any)
	wellChild := well["childGridId"].(string)
	warmT := rpc(t, localOrigin, "CreateTile", map[string]any{
		"gridId": wellChild,
		"tile":   map[string]any{"kind": "text", "x": 0, "y": 0, "w": 1, "h": 1},
	})["tile"].(map[string]any)
	if _, err := cl.WriteContent(ctx, warmT["id"].(string), num(warmT["version"]), []byte("warmed words")); err != nil {
		t.Fatal(err)
	}
	coldT := rpc(t, localOrigin, "CreateTile", map[string]any{
		"gridId": wellChild,
		"tile":   map[string]any{"kind": "text", "x": 2, "y": 0, "w": 1, "h": 1},
	})["tile"].(map[string]any)
	if _, err := cl.WriteContent(ctx, coldT["id"].(string), num(coldT["version"]), []byte("cold words")); err != nil {
		t.Fatal(err)
	}
	// A second never-read text, for the re-warm after the revival: this one is
	// not read even then, so what warms it can only be the walk.
	colderT := rpc(t, localOrigin, "CreateTile", map[string]any{
		"gridId": wellChild,
		"tile":   map[string]any{"kind": "text", "x": 4, "y": 0, "w": 1, "h": 1},
	})["tile"].(map[string]any)
	if _, err := cl.WriteContent(ctx, colderT["id"].(string), num(colderT["version"]), []byte("colder words")); err != nil {
		t.Fatal(err)
	}

	// WARM the cache: the grids and exactly one body. (Writes are never
	// cached — only these reads are.)
	rpc(t, localOrigin, "GetGrid", map[string]any{"gridId": personalChild})
	rpc(t, localOrigin, "GetGrid", map[string]any{"gridId": wellChild})
	if body, _, _, err := cl.ReadContent(ctx, warmT["id"].(string)); err != nil || string(body) != "warmed words" {
		t.Fatalf("warm read = %q (%v)", body, err)
	}
	if _, err := os.Stat(filepath.Join(localHome, "cache.db")); err != nil {
		t.Fatalf("source cache file missing (the node wiring): %v", err)
	}

	// ── THE PARTITION ──────────────────────────────────────────────────
	stopRemote()

	// Warmed reads serve STALE (poll: the dial layer needs a beat to start
	// answering Unavailable instead of hanging on half-open sockets).
	deadline := time.Now().Add(60 * time.Second)
	var staleBody []byte
	var err error
	for time.Now().Before(deadline) {
		staleBody, _, _, err = cl.ReadContent(ctx, warmT["id"].(string))
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil || string(staleBody) != "warmed words" {
		t.Fatalf("dark warmed read = %q (%v), want the cached bytes", staleBody, err)
	}
	// The memory says so on the wire (#256): the cache-served grid wears
	// the stale bit through the whole real chain — sourcecache → server →
	// Connect JSON — which is what the client's offline chip reads. Polled,
	// because "this serve is a memory" waits on the node LEARNING that the
	// connection is dark: a call of its own failing transport-shaped, or the
	// connection's health saying so on the event stream. The grid was read
	// seconds ago, so its own age says nothing yet.
	deadline = time.Now().Add(60 * time.Second)
	var g map[string]any
	for {
		g = rpc(t, localOrigin, "GetGrid", map[string]any{"gridId": wellChild})
		if gm, ok := g["grid"].(map[string]any); ok && gm["stale"] == true {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dark grid read grid=%v, want stale=true on the wire", g["grid"])
		}
		time.Sleep(500 * time.Millisecond)
	}
	if len(g["tiles"].([]any)) != 3 {
		t.Fatalf("dark grid read has %d tiles, want the cached 3", len(g["tiles"].([]any)))
	}
	// A never-read body fails HONESTLY — served-wrong would be worse than
	// unavailable. Both of them: colderT failing here is also what says the
	// establishment walk never reached it, so the re-warm below can only be
	// the recovery's doing.
	if _, _, _, err := cl.ReadContent(ctx, coldT["id"].(string)); err == nil {
		t.Fatal("dark read of never-cached bytes must fail, not fabricate")
	}
	if _, _, _, err := cl.ReadContent(ctx, colderT["id"].(string)); err == nil {
		t.Fatal("the second never-read body was already cached before any recovery")
	}
	awaitConnHealth(t, health, "partconn1", false)

	// The OFFLINE DEEP COPY (the owner-decision scenario, end to end over
	// real binaries): right-drag the remote well into the local plugin
	// while the mount is dark. Cached text → real copy; never-read text →
	// LINK to the original.
	copyResp := rpc(t, localOrigin, "CloneTile", map[string]any{
		"tileId": well["id"], "version": 0, "destGridId": homeRoot, "x": 5, "y": 5,
	})["tile"].(map[string]any)
	if copyResp["reference"] == true {
		t.Fatal("the offline copy's top well must be SOLID (its grid was cached)")
	}
	cg := rpc(t, localOrigin, "GetGrid", map[string]any{"gridId": copyResp["childGridId"]})
	var gotCopy map[string]any
	linked := map[string]bool{}
	for _, ti := range cg["tiles"].([]any) {
		tm := ti.(map[string]any)
		if lt, _ := tm["linkTargetId"].(string); lt != "" {
			linked[lt] = true
		} else if tm["kind"] == "text" {
			gotCopy = tm
		}
	}
	if gotCopy == nil || len(linked) != 2 {
		t.Fatalf("offline copy shape wrong (want one solid text + two links): %v", cg["tiles"])
	}
	if body, _, _, err := cl.ReadContent(ctx, gotCopy["id"].(string)); err != nil || string(body) != "warmed words" {
		t.Fatalf("offline-copied body = %q (%v)", body, err)
	}
	if !linked[coldT["id"].(string)] || !linked[colderT["id"].(string)] {
		t.Fatalf("offline links target %v, want the two never-read originals", linked)
	}

	// ── THE REVIVAL ────────────────────────────────────────────────────
	// Same address, same DB: the connection self-heals (sshdial backoff
	// caps at 10s) and the cold body reads LIVE — proof the cache answers
	// only when the mount cannot.
	_, _, stop2 := startServeProc(t, bin, remoteHome, strings.TrimPrefix(remoteOrigin, "http://"))
	if stop2 == nil {
		t.Fatal("remote revival failed")
	}
	deadline = time.Now().Add(60 * time.Second)
	healed := false
	for time.Now().Before(deadline) {
		if body, _, _, rerr := cl.ReadContent(ctx, coldT["id"].(string)); rerr == nil {
			if string(body) != "cold words" {
				t.Fatalf("revived read = %q, want the live bytes", body)
			}
			healed = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !healed {
		t.Fatal(fmt.Sprintf("the mount never healed after revival on %s", remoteAddr))
	}

	// ── THE RE-WARM (#275) ─────────────────────────────────────────────
	// A recovered connection re-kicks the source's prefetch walk, so the
	// promise — the cache holds a recent copy of what you did NOT happen to
	// read — is true again without a restart. `colderT` is never read live,
	// so the only thing that can warm it is that walk. The health-up on this
	// stream passed through the cache's own arm on its way here, which is the
	// kick; what follows it is a handful of small reads.
	awaitConnHealth(t, health, "partconn1", true)
	time.Sleep(20 * time.Second)
	stop2()
	deadline = time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if body, _, _, rerr := cl.ReadContent(ctx, colderT["id"].(string)); rerr == nil {
			if string(body) != "colder words" {
				t.Fatalf("re-warmed read = %q, want the cached bytes", body)
			}
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("a second partition could not serve the bytes nobody read: the recovery did not re-walk the source")
}
