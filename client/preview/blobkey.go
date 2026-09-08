package preview

import (
	pb "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
	"github.com/josephburnett/gridwell/api/rpc"
)

// PageBlobID is the cache key for a page tile's preview. Page tiles have no
// preview_blob_id, because the owning plugin derives the frozen face from the
// content itself, so there is no generation counter to key freshness by. A
// fixed sentinel fetches once per session.
const PageBlobID = -1

// BlobKey resolves a tile's cache key, 0 meaning no preview and no fetch. The
// one keying rule for every preview draw and fetch.
func BlobKey(t *pb.Tile) int64 {
	if t.PreviewBlobId != 0 {
		return t.PreviewBlobId
	}
	// rpc.PageContent, not the serves_page bit: a url tile is never a page
	// however it is flagged, and keying one to the sentinel would hand it a
	// face the /content/ door never serves.
	if rpc.PageContent(t) {
		return PageBlobID
	}
	return 0
}
