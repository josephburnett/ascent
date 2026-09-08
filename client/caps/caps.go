// Package caps owns the client's environment capability set. The one
// difference between the Electron host and a plain browser is native live URL
// views; shells ride the web door on both. It is derived once at boot, so no
// other code tests for the bridge to make a feature decision. Like
// client/pluginhealth it also holds the errsurface report for a missing one.
package caps

import "github.com/josephburnett/gridwell/client/errsurface"

// Caps is derived once at boot and immutable after.
type Caps struct {
	LiveURL   bool
	LiveShell bool
	// Shells is whether shell tiles exist on this node at all. Without them
	// the + palette offers no shell primitive and the server refuses creates.
	Shells bool
}

// Bridge is what the native host declares it can do, window.gridwell's caps
// field. A host declares each half it implements, so one can place live url
// views without the rest of the desktop. Shells are not on the list because
// the PTY rides the web door.
type Bridge struct {
	Present bool
	// LiveURL is whether the host implements placeWebview and setBounds.
	LiveURL bool
}

// LegacyBridge is what a bridge with no caps field is taken to declare.
func LegacyBridge() Bridge {
	return Bridge{Present: true, LiveURL: true}
}

// NoBridge is a plain browser host.
func NoBridge() Bridge { return Bridge{} }

// Derive runs once before the handshake with shellsDisabled unknown and again
// when that fact lands, and not after.
func Derive(bridge Bridge, shellsDisabled bool) Caps {
	return Caps{
		LiveURL:   bridge.Present && bridge.LiveURL,
		LiveShell: !shellsDisabled,
		Shells:    !shellsDisabled,
	}
}

// GoLiveNotice reports a gesture that asked for a live view without LiveURL. A
// missing capability is expected, so Info, and the source coalesces taps.
func GoLiveNotice() (sev errsurface.Severity, source, message string) {
	return errsurface.Info, "livecap", "live web views need the desktop app — showing the frozen preview"
}

// ShellNotice is GoLiveNotice for a shell tile, on the same source so mixed
// taps coalesce into one row.
func ShellNotice() (sev errsurface.Severity, source, message string) {
	return errsurface.Info, "livecap", "this node has shells turned off — showing the frozen preview"
}
