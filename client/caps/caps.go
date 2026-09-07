// Package caps owns the client's environment capability set. The same wasm
// client runs under the Electron desktop app and under a plain browser, and the
// only difference between them is native live URL views: shells ride the web
// door on every host. That difference is derived once at boot and read
// everywhere, so no other code tests for the bridge to make a feature decision.
//
// Like client/pluginhealth, the package also holds the errsurface report for a
// gesture that hits a missing capability.
package caps

import "github.com/josephburnett/gridwell/client/errsurface"

// Caps is this client's capability set, derived once at boot and immutable
// after.
type Caps struct {
	// LiveURL is whether a URL tile can go live as a native browser view. Only
	// the Electron shell hosts one; a plain browser shows the frozen preview.
	LiveURL bool
	// LiveShell is whether a shell tile can attach its live PTY. The PTY rides a
	// WebSocket on the web door, so it is true wherever this client runs and
	// only the node refusing shells (server.yaml disable_shells) turns it off.
	LiveShell bool
	// Shells is whether shell tiles exist on this node at all. Under
	// shells_disabled the + palette offers no shell primitive and the server
	// refuses shell creates and PTY attaches whatever a client asks.
	Shells bool
}

// Bridge is what the native host declares it can do, the window.gridwell
// object's caps field, read once at boot. A host declares each half it
// implements, so one can place live url views without the rest of the desktop's
// machinery. Shells are not on the list, because the PTY rides the web door.
type Bridge struct {
	// Present is whether window.gridwell exists at all.
	Present bool
	// LiveURL is whether the host implements placeWebview and setBounds.
	LiveURL bool
}

// LegacyBridge is the declaration imputed to a bridge with no caps field, the
// full Electron feature set.
func LegacyBridge() Bridge {
	return Bridge{Present: true, LiveURL: true}
}

// NoBridge is a plain browser host.
func NoBridge() Bridge { return Bridge{} }

// Derive computes the capability set from what the native bridge declares and
// the handshake's shells_disabled. It runs once before the handshake with the
// node fact unknown and again when that fact lands, and not after.
func Derive(bridge Bridge, shellsDisabled bool) Caps {
	return Caps{
		LiveURL:   bridge.Present && bridge.LiveURL,
		LiveShell: !shellsDisabled,
		Shells:    !shellsDisabled,
	}
}

// GoLiveNotice is the errsurface report for a gesture that asked a URL tile to
// go live when LiveURL is false. A missing capability is expected of the host,
// so the severity is Info, and the stable source coalesces repeated taps.
func GoLiveNotice() (sev errsurface.Severity, source, message string) {
	return errsurface.Info, "livecap", "live web views need the desktop app — showing the frozen preview"
}

// ShellNotice is the errsurface report for a gesture that asked a shell tile to
// attach when LiveShell is false. Severity and source follow GoLiveNotice, so
// mixed taps coalesce into one capability row.
func ShellNotice() (sev errsurface.Severity, source, message string) {
	return errsurface.Info, "livecap", "this node has shells turned off — showing the frozen preview"
}
