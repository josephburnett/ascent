//go:build js && wasm

package shellws

import (
	"context"

	"github.com/coder/websocket"
)

// dialConn opens the browser's own WebSocket. The handshake is a same-origin
// page request, so the page's auth cookie rides it, and a browser forbids
// setting handshake headers at all.
func dialConn(ctx context.Context, addr string, _ Options) (*websocket.Conn, error) {
	ws, _, err := websocket.Dial(ctx, addr, nil)
	return ws, err
}
