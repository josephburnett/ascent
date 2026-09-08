package server

// The node export: the same router the browser talks to, re-served over raw
// gRPC on the connection door. Ids compose across hops because the router peels
// exactly one segment per request and prepends exactly one per response, so
// there is no name-based selection and no scoping header.

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"

	pb "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
	"github.com/josephburnett/gridwell/internal/namespace"
)

// WebHandler is the browser door: static files, Connect RPCs, the /shell
// socket and the /content/ pages, behind the password gate in auth.go. Raw
// gRPC is not demuxed here, so binding `web.bind` to a network address exposes
// exactly the gated surface and nothing else.
func (s *Server) WebHandler() http.Handler { return s.authWrap(s.mux) }

// WebDoorServer is the web door's one server shape, so the production node and
// every test harness put the same server in front of the browser handler; the
// node sets BaseContext per its own listener. ReadHeaderTimeout stays here
// alone, because this door faces a network and carries no raw-gRPC stream for
// a deadline to cut. No Protocols: it refuses raw gRPC by design
// (TestWebDoorServesNoGRPC).
func WebDoorServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

// ConnectionHandler is the connection door: the Gridwell service over raw gRPC,
// what a remote mounter's ssh tunnel dials. Its gate is the kernel, the 0600
// unix socket ListenConnectionDoor opens, and ssh is the authenticated
// transport between nodes. Serve it with ConnectionDoorServer. It and
// internal/connection/dial are the connection hop's two ends.
func (s *Server) ConnectionHandler() http.Handler {
	g := grpc.NewServer()
	pb.RegisterGridwellServer(g, namespace.Server(newRouter(s)))
	return g
}

// ConnectionDoorServer is the connection door's one server shape, so a test
// that holds a stream through it holds it through what the node runs.
//
// No deadline of any kind, deliberately. net/http arms ReadHeaderTimeout on the
// raw conn before handing it to the HTTP/2 server (Go 1.26.6), whose only
// disarm is tied to ReadTimeout, and WriteTimeout becomes a per-stream deadline
// there too, so any deadline is a ticking close on every long-lived gRPC stream
// through this door. A slow-header peer is no concern on a 0600 unix socket,
// and gRPC keepalive polices a silent one.
func ConnectionDoorServer(h http.Handler) *http.Server {
	return &http.Server{Handler: h, Protocols: NodeProtocols()}
}

// ListenConnectionDoor opens the connection door's one listener shape: a 0600
// unix socket, whose mode is the door's whole gate. It unlinks a stale socket
// from a crashed serve first; the serve lock guarantees no live holder.
func ListenConnectionDoor(path string) (net.Listener, error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("connection door: %w", err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("connection door: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("connection door: %w", err)
	}
	return ln, nil
}

// NodeProtocols is the protocol set for the connection door: HTTP/1.1 plus
// unencrypted HTTP/2, because the ssh tunnel is already private and TLS-only
// h2 would refuse the mounter.
func NodeProtocols() *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	return p
}
