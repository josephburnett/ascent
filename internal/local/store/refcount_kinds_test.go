package store

import (
	"context"
	"testing"

	gridwellv1 "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
	"github.com/josephburnett/gridwell/api/rpc"
)

// blobExists reports whether a blob row is still present (refcount > 0
// rows are kept; a fully-released blob is DELETEd).
func blobExists(t *testing.T, s *Store, id int64) bool {
	t.Helper()
	var n int64
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM blobs WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// A PTY cannot be forked, so cloning a shell with a frozen preview carries the
// preview blob to the clone and bumps that blob's refcount.
func TestCloneShellCarriesScreenshot(t *testing.T) {
	s := newTestStore(t)
	root := rootID(t, s)
	ctx := context.Background()

	sh, err := s.CreateShell(ctx, root, 0, 0, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	framed, err := s.SetShellPreview(ctx, sh.Id, []byte("frozen-shell"))
	if err != nil {
		t.Fatal(err)
	}
	if framed.PreviewBlobId == 0 {
		t.Fatal("shell has no preview blob after SetShellPreview")
	}

	clone, err := s.CloneTile(ctx, &gridwellv1.CloneTileRequest{
		TileId:     framed.Id,
		DestGridId: root, X: 50, Y: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if clone.Kind != rpc.KindShell {
		t.Errorf("clone kind = %q, want shell", clone.Kind)
	}
	if clone.PreviewBlobId != framed.PreviewBlobId {
		t.Errorf("clone preview blob = %d, want %d (screenshot must be carried)",
			clone.PreviewBlobId, framed.PreviewBlobId)
	}
	// Two tiles now reference the one preview blob.
	verifyRefcounts(t, s)
}

// cloneSubtree bumps the preview blob for every copied tile that holds one, so
// a deep copy of a shell-with-preview leaves that blob at refcount 2.
func TestCloneCopiesShellPreviewBlob(t *testing.T) {
	s := newTestStore(t)
	root := rootID(t, s)
	ctx := context.Background()

	well, err := s.CreateWell(ctx, root, 0, 0, 1, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	sh, err := s.CreateShell(ctx, well.ChildGridId, 0, 0, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetShellPreview(ctx, sh.Id, []byte("frozen-shell")); err != nil {
		t.Fatal(err)
	}

	// Clone the well: copy-on-clone deep-copies its child grid, re-rowing the
	// shell-with-preview. The copy shares the immutable preview blob, so its
	// refcount must rise to 2 — the case cloneSubtree must get right.
	clone, err := s.CloneTile(ctx, &gridwellv1.CloneTileRequest{
		TileId:     well.Id,
		DestGridId: root, X: 50, Y: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Two shell rows (original + copy) now share one preview blob.
	cloneChild, err := s.GetGrid(ctx, clone.ChildGridId)
	if err != nil {
		t.Fatal(err)
	}
	if len(cloneChild.Tiles) != 1 || cloneChild.Tiles[0].Kind != rpc.KindShell {
		t.Fatalf("clone child = %+v, want one shell tile", cloneChild.Tiles)
	}
	previewBlob := cloneChild.Tiles[0].PreviewBlobId
	if previewBlob == 0 {
		t.Fatal("cloned shell lost its preview blob")
	}
	if rc := refcount(t, s, "blobs", previewBlob); rc != 2 {
		t.Errorf("preview blob refcount = %d, want 2", rc)
	}
	verifyRefcounts(t, s)
}

// TestDeleteGridReleasesAllKindRefs: GC'ing a grid (its last well deleted)
// must release every reference its tiles held — in particular a shell's
// preview blob — so deleting the containing well doesn't leak the blob.
func TestDeleteGridReleasesAllKindRefs(t *testing.T) {
	s := newTestStore(t)
	root := rootID(t, s)
	ctx := context.Background()

	well, err := s.CreateWell(ctx, root, 0, 0, 1, 1, "")
	if err != nil {
		t.Fatal(err)
	}

	sh, err := s.CreateShell(ctx, well.ChildGridId, 2, 0, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	framed, err := s.SetShellPreview(ctx, sh.Id, []byte("frozen-shell"))
	if err != nil {
		t.Fatal(err)
	}
	previewBlob := framed.PreviewBlobId
	if previewBlob == 0 {
		t.Fatalf("setup: previewBlob=%d", previewBlob)
	}

	// Destroy the only well pointing at the child grid, collapsing the
	// two-stage gesture: the child grid is collected, cascading through the
	// shell inside it.
	hardDelete(t, s, well.Id)

	verifyRefcounts(t, s)
	if blobExists(t, s, previewBlob) {
		t.Errorf("shell preview blob %d survived GC — refcount leaked", previewBlob)
	}
}
