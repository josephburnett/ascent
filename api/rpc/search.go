package rpc

import (
	"context"
	"strings"
	"time"

	"connectrpc.com/connect"

	pb "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
)

// SearchHopTimeout bounds one hop's answer during a search fan-out, so a hung
// hop cannot stall the whole search. One owner for both fan-outs: the
// server's per-plugin loop and the connection transport's per-connection one.
const SearchHopTimeout = 3 * time.Second

// SearchQuery is the parsed form of a Search query, the one grammar every
// plugin reads. Exactly one of ID and Text is set.
type SearchQuery struct {
	// ID is an exact tile id to locate; the result carries the tile and its
	// containing-well path.
	ID string
	// Text is interpreted by each plugin against its own data.
	Text string
}

// ParseSearchQuery is the selector grammar: id:<tile-id> and free text. New
// selectors extend here, never in a plugin, or the grammar forks.
func ParseSearchQuery(q string) SearchQuery {
	q = strings.TrimSpace(q)
	if rest, ok := strings.CutPrefix(q, "id:"); ok {
		return SearchQuery{ID: strings.TrimSpace(rest)}
	}
	return SearchQuery{Text: q}
}

// Search issues one query. scope routes to the namespace owning that qualified
// id; "" fans out across every plugin, and transit nodes recurse. limit caps
// results per answering plugin, 0 for its default.
func (c *Client) Search(ctx context.Context, query, scope string, limit int32) ([]*pb.SearchResult, error) {
	resp, err := c.cl.Search(ctx, connect.NewRequest(&pb.SearchRequest{
		Query: query, Scope: scope, Limit: limit,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Results, nil
}
