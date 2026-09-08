package server

import (
	"context"
	"fmt"
	"github.com/josephburnett/gridwell/api/gwerr"

	pb "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
	"github.com/josephburnett/gridwell/api/rpc"
	"github.com/josephburnett/gridwell/internal/namespace"
)

// Cross-plugin deep copy of a solid well. The router walks the source subtree
// through the namespace interface and materializes it in the destination
// plugin: create, recurse, bytes through the one content door, framing through
// the one writeback.
//
// Creation is necessarily top-down, because an interior well's child grid is
// allocated by its create. So the destination well appears immediately and
// fills in, and a mid-copy failure leaves a partial subtree that is visible and
// deletable with the error surfaced: never a silent half-state, and never a
// rollback that could destroy what the user already sees.
//
// What copies as what:
//   - a solid interior well becomes a new well plus a recursive copy of its
//     child grid, with its framing preserved through SetFraming;
//   - an exit well or leaf link copies as a reference, the top-level clone's
//     rule;
//   - text and pane bytes go ReadContent to WriteContent; a pane layout stays
//     owner-frame-relative, which is cross-plugin link semantics in bytes;
//   - a url copies its url_string plus the frozen preview and history;
//   - a shell becomes a fresh shell, since a PTY session is namespace-local.

// deepCopyWell reads the source child grid before anything is created, so an
// unreachable room degrades to a link (sourceUnreachable with a nil out) rather
// than to an empty solid well pretending to be a copy.
func (rt *router) deepCopyWell(ctx context.Context, src namespace.Namespace, srcTransit bool, srcUUID string, srcLocalTile *pb.Tile, dst namespace.Namespace, dstGrid string, x, y int64) (*pb.TileResponse, error) {
	srcChild := srcLocalTile.ChildGridId
	g, err := src.GetGrid(ctx, &pb.GetGridRequest{GridId: srcChild})
	if err != nil {
		return nil, fmt.Errorf("read source grid %s: %w", srcChild, err)
	}

	created, err := dst.CreateTile(ctx, &pb.CreateTileRequest{
		GridId: dstGrid,
		Tile: &pb.Tile{Kind: rpc.KindWell, X: x, Y: y, W: srcLocalTile.W, H: srcLocalTile.H,
			AltText: srcLocalTile.AltText},
	})
	if err != nil {
		return nil, err
	}
	// The well's framing is at once the preview, the descent target and the
	// ascent return. The writeback's response is the current row, so return
	// that rather than the pre-framing create response.
	framed, err := dst.SetFraming(ctx, &pb.SetFramingRequest{
		TileId: created.GetTile().GetId(),
		Cx:     srcLocalTile.ViewCx, Cy: srcLocalTile.ViewCy, Zoom: srcLocalTile.ViewZoom,
	})
	if err != nil {
		return created, fmt.Errorf("well framing: %w", err)
	}
	created = &pb.TileResponse{Tile: framed.GetTile()}

	dstChild := created.GetTile().GetChildGridId()
	for _, child := range g.Tiles {
		if err := rt.deepCopyTile(ctx, src, srcTransit, srcUUID, child, dst, dstChild); err != nil {
			return created, fmt.Errorf("copy tile %s: %w", child.Id, err)
		}
	}
	return created, nil
}

// deepCopyTile copies one source tile into dstGrid at the source's own
// coordinates.
func (rt *router) deepCopyTile(ctx context.Context, src namespace.Namespace, srcTransit bool, srcUUID string, t *pb.Tile, dst namespace.Namespace, dstGrid string) error {
	// The qualification every wire response gets decides whether this tile is
	// a reference, so the copy and a fresh read cannot disagree.
	q := qualifyTilesFor(srcTransit, srcUUID, []*pb.Tile{t})[0]

	switch {
	case rpc.IsWellKind(q.Kind) && q.Reference:
		// A reference copies as a reference: the shared child, qualified.
		_, err := dst.CreateTile(ctx, &pb.CreateTileRequest{
			GridId: dstGrid,
			Tile: &pb.Tile{Kind: rpc.KindWell, X: t.X, Y: t.Y, W: t.W, H: t.H,
				AltText: t.AltText, ChildGridId: q.ChildGridId,
				ViewCx: t.ViewCx, ViewCy: t.ViewCy, ViewZoom: t.ViewZoom},
		})
		return err
	case rpc.IsWellKind(q.Kind):
		created, err := rt.deepCopyWell(ctx, src, srcTransit, srcUUID, t, dst, dstGrid, t.X, t.Y)
		// Degrade only when nothing was created. Degrading with a partial in
		// place would stack a link on the cell it occupies, and the user
		// would get an overlap refusal on a grid they never touched.
		if created == nil && gwerr.IsTransport(err) {
			// The room is dark, not gone, so degrade to a link: the dashed
			// border already means "lives elsewhere", which beats failing the
			// walk or leaving an empty well that lies about being a copy.
			_, lerr := dst.CreateTile(ctx, &pb.CreateTileRequest{
				GridId: dstGrid,
				Tile: &pb.Tile{Kind: rpc.KindWell, X: t.X, Y: t.Y, W: t.W, H: t.H,
					AltText: t.AltText, ChildGridId: q.ChildGridId,
					ViewCx: t.ViewCx, ViewCy: t.ViewCy, ViewZoom: t.ViewZoom},
			})
			return lerr
		}
		return err
	case q.LinkTargetId != "":
		_, err := dst.CreateTile(ctx, &pb.CreateTileRequest{
			GridId: dstGrid,
			Tile: &pb.Tile{Kind: t.Kind, X: t.X, Y: t.Y, W: t.W, H: t.H,
				AltText: t.AltText, LinkTargetId: q.LinkTargetId},
		})
		return err
	}

	// Leaf bytes are read before the copy row is created, so an unreachable
	// source degrades to a link instead of an empty copy that looks whole.
	var body []byte
	if rpc.IsBodyKind(t.Kind) && t.BlobId != 0 {
		var err error
		body, err = readAllContent(ctx, src, t.Id)
		if gwerr.IsTransport(err) {
			_, lerr := dst.CreateTile(ctx, &pb.CreateTileRequest{
				GridId: dstGrid,
				Tile: &pb.Tile{Kind: t.Kind, X: t.X, Y: t.Y, W: t.W, H: t.H,
					AltText: t.AltText, LinkTargetId: q.Id},
			})
			return lerr
		}
		if err != nil {
			return err
		}
	}

	created, err := dst.CreateTile(ctx, &pb.CreateTileRequest{
		GridId: dstGrid,
		Tile: &pb.Tile{Kind: t.Kind, X: t.X, Y: t.Y, W: t.W, H: t.H,
			AltText: t.AltText, UrlString: t.UrlString},
	})
	if err != nil {
		return err
	}
	id := created.GetTile().GetId()
	version := created.GetTile().GetVersion()

	switch {
	case rpc.IsBodyKind(t.Kind):
		if len(body) == 0 {
			return nil
		}
		_, err = writeAllContent(ctx, dst, id, version, body)
		return err
	case t.Kind == rpc.KindURL || t.Kind == rpc.KindShell:
		// The frozen face travels with the copy. An unreachable preview skips:
		// the copy's own facts are present and the face re-freezes on the next
		// live visit, so a link here would deny the copy content the walk has.
		if t.PreviewBlobId == 0 {
			return nil
		}
		pv, err := src.GetTilePreview(ctx, &pb.GetTilePreviewRequest{TileId: t.Id})
		if gwerr.IsTransport(err) {
			return nil
		}
		if err != nil || len(pv.GetJpeg()) == 0 {
			return err
		}
		_, err = dst.SetTile(ctx, &pb.SetTileRequest{
			TileId: id, Version: version,
			Tile:    &pb.Tile{Kind: t.Kind, UrlString: t.UrlString, UrlHistory: t.UrlHistory},
			Preview: pv.GetJpeg(),
		})
		return err
	}
	return nil
}
