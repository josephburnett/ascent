// Package compose is the plugin door this repository owns: the go-plugin
// handshake both sides present, and the host-side spawn in plugin.go. A
// plugin is always an out-of-process binary, so third-party code runs with
// its own dependency graph. The guest-side helper a plugin's main() calls,
// and every plugin, live in github.com/josephburnett/gridwell-plugins.
package compose

import (
	"github.com/hashicorp/go-plugin"
)

// HandshakeConfig is what the host and the plugin process exchange at
// startup. A mismatch makes the host refuse the connection.
var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "GRIDWELL_PLUGIN",
	MagicCookieValue: "gridwell-plugin-v1",
}
