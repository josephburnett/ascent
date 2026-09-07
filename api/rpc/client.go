package rpc

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	pb "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
	"github.com/josephburnett/gridwell/api/gen/gridwell/v1/gridwellv1connect"
)

// Client wraps the Connect-generated gridwell client: one method per verb,
// streams assembled, and the typed sugar over the unified CreateTile and
// SetTile. It accepts and returns the generated proto values. Tests, the
// WASM client, and any future Go callers should use this rather than the raw
// connect client.
type Client struct {
	cl gridwellv1connect.GridwellClient
}

// NewClient wires a Client to a Connect-RPC server reachable via the
// given HTTP client + base URL. baseURL is the protocol + host, with
// no path (e.g. "http://localhost:3137"); Connect appends the
// per-method procedure path.
func NewClient(httpClient connect.HTTPClient, baseURL string, opts ...connect.ClientOption) *Client {
	return &Client{cl: gridwellv1connect.NewGridwellClient(httpClient, baseURL, opts...)}
}

// NewDefaultClient is the WASM-friendly constructor: it uses
// http.DefaultClient (under WASM this rides on fetch via syscall/js)
// and Connect's JSON-over-proto codec so dev-tools network panels
// still show readable bodies.
func NewDefaultClient(baseURL string) *Client {
	return NewClient(http.DefaultClient, baseURL, connect.WithProtoJSON())
}

func (c *Client) GetGrid(ctx context.Context, gridID string) (*pb.GetGridResponse, error) {
	r, err := c.cl.GetGrid(ctx, connect.NewRequest(&pb.GetGridRequest{GridId: gridID}))
	if err != nil {
		return nil, err
	}
	return r.Msg, nil
}

func (c *Client) GetTilePreview(ctx context.Context, tileID string) ([]byte, error) {
	r, err := c.cl.GetTilePreview(ctx, connect.NewRequest(&pb.GetTilePreviewRequest{TileId: tileID}))
	if err != nil {
		return nil, err
	}
	return r.Msg.Jpeg, nil
}

func (c *Client) Handshake(ctx context.Context) (*pb.HandshakeResponse, error) {
	return c.HandshakeNS(ctx, "")
}

// HandshakeNS is the routed plugin list. ns "" answers for the node this
// client talks to (the boot handshake); a namespace chain answers for the
// node it names, with ids re-qualified per hop and node-local fields
// zeroed. The + menu inside a remote pane is built from this.
func (c *Client) HandshakeNS(ctx context.Context, ns string) (*pb.HandshakeResponse, error) {
	r, err := c.cl.Handshake(ctx, connect.NewRequest(&pb.HandshakeRequest{Namespace: ns}))
	if err != nil {
		return nil, err
	}
	return r.Msg, nil
}

// GetTile reads a single tile's metadata by id.
func (c *Client) GetTile(ctx context.Context, tileID string) (*pb.Tile, error) {
	r, err := c.cl.GetTile(ctx, connect.NewRequest(&pb.GetTileRequest{TileId: tileID}))
	if err != nil {
		return nil, err
	}
	return r.Msg.Tile, nil
}

// tileResp unwraps a TileResponse from any of the Tile-returning RPCs (or
// the transport error). The mirror of the server's tileResp: every Create /
// Move / Clone / Resize / Set / Update method ends the same way, so the
// unwrap lives in one place rather than being hand-copied per method.
func tileResp(r *connect.Response[pb.TileResponse], err error) (*pb.Tile, error) {
	if err != nil {
		return nil, err
	}
	return r.Msg.Tile, nil
}

// CreateTile is the one create: the wire carries a single CreateTile whose
// tile.kind selects the meaningful fields, and the serving namespace fans it
// back out.
func (c *Client) CreateTile(ctx context.Context, req *pb.CreateTileRequest) (*pb.Tile, error) {
	return tileResp(c.cl.CreateTile(ctx, connect.NewRequest(req)))
}

// CreateWithContent makes the metadata row and, when data is set, follows
// with the one content write. Creation is metadata-only on the wire; this
// composes the two so a text tile or a pane tile can be made in one call. A
// failure between the two leaves an empty tile — visible and deletable,
// never silent.
func (c *Client) CreateWithContent(ctx context.Context, req *pb.CreateTileRequest, data []byte) (*pb.Tile, error) {
	t, err := c.CreateTile(ctx, req)
	if err != nil || len(data) == 0 {
		return t, err
	}
	return c.WriteContent(ctx, t.Id, t.Version, data)
}

// SetTile is the one capture and framing writeback: tile.kind selects the
// operation, and the scalar arms (rename, content_zoom, url_frozen) carry
// exactly one operation per call.
func (c *Client) SetTile(ctx context.Context, req *pb.SetTileRequest) (*pb.Tile, error) {
	return tileResp(c.cl.SetTile(ctx, connect.NewRequest(req)))
}

// SetFraming persists a grid's framing: the one framing write. The server
// routes on whichever target the request names, the doorway tile or the
// root grid. Returns the updated doorway tile, or nil for a root, which has
// no tile row.
func (c *Client) SetFraming(ctx context.Context, req *pb.SetFramingRequest) (*pb.Tile, error) {
	resp, err := c.cl.SetFraming(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetTile(), nil
}

// ReadContent fetches a tile's content bytes: the one content read. The
// stream is assembled here, and chunk 1 carries the media type and the row
// version the bytes belong to — the save basis, paired with the bytes at
// the owner. A leaf link resolves to its target at the serving node, so
// callers never reimplement link semantics.
func (c *Client) ReadContent(ctx context.Context, tileID string) (data []byte, mediaType string, version int64, err error) {
	stream, err := c.cl.ReadContent(ctx, connect.NewRequest(&pb.ReadContentRequest{TileId: tileID}))
	if err != nil {
		return nil, "", 0, err
	}
	defer stream.Close()
	first := true
	for stream.Receive() {
		msg := stream.Msg()
		if first {
			mediaType, version = msg.MediaType, msg.Version
			first = false
		}
		data = append(data, msg.Data...)
	}
	if err := stream.Err(); err != nil {
		return nil, "", 0, err
	}
	return data, mediaType, version, nil
}

// writeContentChunkBytes bounds each upload message; the server reassembles
// and commits once, at clean close.
const writeContentChunkBytes = 256 * 1024

// WriteContent writes a tile's content bytes: the one content write. It is
// version-claimed and commits at close, so a failure anywhere leaves the
// old value intact. data is the complete new value; chunking is a transport
// detail.
func (c *Client) WriteContent(ctx context.Context, tileID string, version int64, data []byte) (*pb.Tile, error) {
	stream := c.cl.WriteContent(ctx)
	end := min(writeContentChunkBytes, len(data))
	if err := stream.Send(&pb.WriteContentRequest{TileId: tileID, Version: version, Data: data[:end]}); err != nil {
		_, cerr := stream.CloseAndReceive()
		if cerr != nil {
			return nil, cerr
		}
		return nil, err
	}
	for off := end; off < len(data); off += writeContentChunkBytes {
		e := min(off+writeContentChunkBytes, len(data))
		if err := stream.Send(&pb.WriteContentRequest{Data: data[off:e]}); err != nil {
			_, cerr := stream.CloseAndReceive()
			if cerr != nil {
				return nil, cerr
			}
			return nil, err
		}
	}
	resp, err := stream.CloseAndReceive()
	if err != nil {
		return nil, err
	}
	return resp.Msg.Tile, nil
}

// PlaceTile is the single placement writeback: one verb owns
// (grid, x, y, w, h), whether that is a move, a resize, or both.
func (c *Client) PlaceTile(ctx context.Context, req *pb.PlaceTileRequest) (*pb.Tile, error) {
	return tileResp(c.cl.PlaceTile(ctx, connect.NewRequest(req)))
}

func (c *Client) CloneTile(ctx context.Context, req *pb.CloneTileRequest) (*pb.Tile, error) {
	return tileResp(c.cl.CloneTile(ctx, connect.NewRequest(req)))
}

// ShellSessionAlive reports whether the tile's tmux session still exists.
func (c *Client) ShellSessionAlive(ctx context.Context, tileID string) (bool, error) {
	r, err := c.cl.ShellSessionAlive(ctx, connect.NewRequest(&pb.ShellSessionAliveRequest{TileId: tileID}))
	if err != nil {
		return false, err
	}
	return r.Msg.Alive, nil
}

// RenameTile is the versioned rename, over the SetTile rename arm. It is a
// real user edit with an optimistic-concurrency claim, and the server
// latches alt_user so automatic captures defer.
func (c *Client) RenameTile(ctx context.Context, tileID string, version int64, alt string) (*pb.Tile, error) {
	return tileResp(c.cl.SetTile(ctx, connect.NewRequest(&pb.SetTileRequest{
		TileId: tileID, Version: version, Rename: alt,
	})))
}

// SetContentZoom persists a tile's content scale — the text or terminal
// font size, or the page zoom. It is framing: no claim, and it never bumps
// version. Rides the SetTile content_zoom arm.
func (c *Client) SetContentZoom(ctx context.Context, tileID string, zoom float64) (*pb.Tile, error) {
	return tileResp(c.cl.SetTile(ctx, connect.NewRequest(&pb.SetTileRequest{
		TileId: tileID, ContentZoom: &zoom,
	})))
}

// SetURLFrozen persists the user's standing freeze on a url tile. It is
// framing: no claim, and it never bumps version. Rides the SetTile
// url_frozen arm.
func (c *Client) SetURLFrozen(ctx context.Context, tileID string, frozen bool) (*pb.Tile, error) {
	return tileResp(c.cl.SetTile(ctx, connect.NewRequest(&pb.SetTileRequest{
		TileId: tileID, UrlFrozen: &frozen,
	})))
}

func (c *Client) DeleteTile(ctx context.Context, req *pb.DeleteTileRequest) error {
	_, err := c.cl.DeleteTile(ctx, connect.NewRequest(req))
	return err
}

// EventStream is the typed wrapper around Connect's server-stream
// client. Recv blocks until the next event arrives, or returns false
// if the stream ended cleanly. Always call Close.
type EventStream struct {
	s *connect.ServerStreamForClient[pb.Event]
}

// Subscribe opens the event stream. The returned context-tied stream
// closes when ctx is cancelled.
func (c *Client) Subscribe(ctx context.Context) (*EventStream, error) {
	s, err := c.cl.Subscribe(ctx, connect.NewRequest(&pb.SubscribeRequest{}))
	if err != nil {
		return nil, err
	}
	return &EventStream{s: s}, nil
}

// Recv returns the next event, or (nil, false, nil) on clean end-of-stream.
// Errors during read surface as (nil, false, err).
func (s *EventStream) Recv() (*pb.Event, bool, error) {
	if !s.s.Receive() {
		return nil, false, s.s.Err()
	}
	return s.s.Msg(), true, nil
}

// Close releases the underlying HTTP connection.
func (s *EventStream) Close() error { return s.s.Close() }
