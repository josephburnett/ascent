// Package namespace is the node's in-process interface. One namespace, whether
// the home store, a content plugin's adapter, or a connection to another node,
// is a Go value the router calls directly. The two codecs here carry the same
// method set onto the two wires that remain.
//
// The four gRPC stream shapes become ordinary Go control flow, decided once
// here so no implementation invents its own:
//
//   - unary:            (ctx, *Request) (*Response, error)
//   - server-streaming: (ctx, *Request, send func(*Chunk) error) error
//   - client-streaming: (ctx, recv func() (*Msg, error)) (*Response, error)
//   - bidirectional:    (ctx, recv func() (*Msg, error), send func(*Msg) error) error
//
// recv reports the end of the caller's messages with io.EOF, exactly as a gRPC
// server stream's Recv does. A send that returns an error aborts the call with
// that error. The caller's ctx is what ends a stream nobody is reading any more.
//
// # Errors
//
// Errors are gRPC status errors (google.golang.org/grpc/status), always. The
// client classifies by code (client/clientsync), so the code is part of the
// contract: it must read the same whether the answer came from a Go call, the
// Connect codec, or two connection hops away. Both codecs in this package
// preserve codes, and the Connect handler maps them through gwerr's one table
// (server.asConnectError).
//
// # Message ownership
//
// Without a wire between them, a caller and a Namespace share the proto
// messages they pass. The contract, which is why no copy layer is needed:
//
//   - a Namespace must not retain or mutate a request after it returns; one
//     that rewrites ids clones first, as internal/connection does;
//   - a Namespace must not mutate a response after returning it;
//   - a caller must not mutate a response in place. The qualification layer
//     clones (api/rpc.TransitQualifyTiles, server.qualifyTiles) so that two
//     subscribers of the same event never see each other's prefix.
package namespace

import (
	"context"

	pb "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
)

// Namespace is one addressable space of grids and tiles, in-process. It is the
// gridwell.v1 service's method set in Go. The router (internal/server) resolves
// a qualified id to a Namespace and calls it; FromClient reads that method set
// off a wire and Server writes it onto one.
type Namespace interface {
	// ── identity and capabilities ────────────────────────────────────────
	Info(ctx context.Context, req *pb.InfoRequest) (*pb.InfoResponse, error)
	Probe(ctx context.Context, req *pb.ProbeRequest) (*pb.ProbeResponse, error)
	Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error)

	// ── reads ────────────────────────────────────────────────────────────
	GetGrid(ctx context.Context, req *pb.GetGridRequest) (*pb.GetGridResponse, error)
	GetTile(ctx context.Context, req *pb.GetTileRequest) (*pb.TileResponse, error)
	GetTilePreview(ctx context.Context, req *pb.GetTilePreviewRequest) (*pb.GetTilePreviewResponse, error)
	Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error)

	// ── writes ───────────────────────────────────────────────────────────
	CreateTile(ctx context.Context, req *pb.CreateTileRequest) (*pb.TileResponse, error)
	SetTile(ctx context.Context, req *pb.SetTileRequest) (*pb.TileResponse, error)
	PlaceTile(ctx context.Context, req *pb.PlaceTileRequest) (*pb.TileResponse, error)
	CloneTile(ctx context.Context, req *pb.CloneTileRequest) (*pb.TileResponse, error)
	DeleteTile(ctx context.Context, req *pb.DeleteTileRequest) (*pb.DeleteTileResponse, error)
	SetFraming(ctx context.Context, req *pb.SetFramingRequest) (*pb.SetFramingResponse, error)

	// ── content ──────────────────────────────────────────────────────────

	// ReadContent streams a tile's bytes. The first chunk carries
	// media_type and the row version the bytes belong to.
	ReadContent(ctx context.Context, req *pb.ReadContentRequest, send func(*pb.ContentChunk) error) error
	// ServeContent streams a plugin-served web response (the /content/
	// door). The first chunk carries status and media_type.
	ServeContent(ctx context.Context, req *pb.ServeContentRequest, send func(*pb.ServeContentChunk) error) error
	// WriteContent assembles the caller's messages and commits once, at a
	// clean io.EOF: a recv that fails leaves the old value byte-for-byte
	// intact. The first message binds tile_id and claims the version.
	WriteContent(ctx context.Context, recv func() (*pb.WriteContentRequest, error)) (*pb.TileResponse, error)

	// ── live ─────────────────────────────────────────────────────────────

	ShellSessionAlive(ctx context.Context, req *pb.ShellSessionAliveRequest) (*pb.ShellSessionAliveResponse, error)
	// OpenShell attaches a tile's PTY. The first recv binds the tile id and
	// the initial size; keystrokes and resizes flow up, terminal output
	// down, until either side ends.
	OpenShell(ctx context.Context, recv func() (*pb.OpenShellRequest, error), send func(*pb.OpenShellResponse) error) error
	// Subscribe streams this namespace's change events until ctx ends.
	Subscribe(ctx context.Context, req *pb.SubscribeRequest, send func(*pb.Event) error) error
}
