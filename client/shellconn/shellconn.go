// Package shellconn holds the decisions the wasm shell attachment makes:
// what a descent does about liveness, whether the refresh button shows, who
// owns a link press, and how a freeze capture decodes. Stream lifecycle lives
// in client/shellstream, over the /shell WebSocket dialer in
// client/shellws.
package shellconn

import "encoding/base64"

// DecodeJPEGDataURL decodes a "data:image/jpeg;base64,..." data URL into raw
// JPEG bytes. ok is false when s lacks the exact prefix or the base64 body
// does not decode.
//
// The decode happens in Go rather than through JS atob, whose binary string
// re-encodes as UTF-8 when read back through js.Value.String() and doubles
// every byte at or above 0x80.
func DecodeJPEGDataURL(s string) ([]byte, bool) {
	const prefix = "data:image/jpeg;base64,"
	if len(s) <= len(prefix) || s[:len(prefix)] != prefix {
		return nil, false
	}
	out, err := base64.StdEncoding.DecodeString(s[len(prefix):])
	if err != nil {
		return nil, false
	}
	return out, true
}

// AutoLive is what a descent into a tile does about liveness. Descending is
// the engagement gesture, so it reconnects the shell and reopens the url.
type AutoLive int

const (
	// AutoLiveNone stays frozen. A host without the capability descends
	// silently, because a notice belongs to an explicit gesture.
	AutoLiveNone AutoLive = iota
	// AutoLiveURL opens the native url view.
	AutoLiveURL
	// AutoLiveShell opens the PTY stream, attaching to a live session or
	// creating one for a tile that has never been opened.
	AutoLiveShell
	// AutoLiveProbeShell probes the session first and decides again from
	// the verdict.
	AutoLiveProbeShell
)

// DecideAutoLive maps a descent's facts to its liveness action. webContent
// comes from rpc.Tile.WebContent(), which classifies a url tile and a
// serves_page tile alike, and the caller feeds it in so this package never
// re-derives it. hasPreview and the aliveness pair are the same facts
// DecideShellRefreshVisible reads, so the two agree about what a dead session
// means. urlFrozen is the user's standing freeze on a url tile, which beats
// the engagement default until the reconnect gesture clears it; a page tile
// carries no standing freeze and passes false.
func DecideAutoLive(webContent, kindShell, liveURL, liveShell, hasPreview, aliveKnown, alive, urlFrozen bool) AutoLive {
	switch {
	case webContent:
		if liveURL && !urlFrozen {
			return AutoLiveURL
		}
	case kindShell:
		if !liveShell {
			return AutoLiveNone
		}
		if !hasPreview {
			return AutoLiveShell // fresh tile: create, as the create path does
		}
		if !aliveKnown {
			return AutoLiveProbeShell
		}
		if alive {
			return AutoLiveShell
		}
	}
	return AutoLiveNone
}

// RefreshVisibility says whether a frozen shell descent's refresh button
// shows, and whether the caller must start a liveness probe.
type RefreshVisibility struct {
	Show  bool
	Probe bool
}

// DecideShellRefreshVisible decides whether the refresh button paints on a
// frozen shell descent and whether a ShellSessionAlive probe must start. A
// tile with no preview blob has never been opened, so refresh creates a
// session and always shows; a session cached dead has no recovery and hides.
// aliveKnown says whether the probe result is cached, and alive is that
// value. The caller runs the probe when Probe is set.
func DecideShellRefreshVisible(isShell, hasPreview, aliveKnown, alive bool) RefreshVisibility {
	if !isShell {
		return RefreshVisibility{}
	}
	if !hasPreview {
		return RefreshVisibility{Show: true}
	}
	if aliveKnown {
		return RefreshVisibility{Show: alive}
	}
	return RefreshVisibility{Probe: true}
}

// MouseTrackingNone is xterm's modes.mouseTrackingMode value for an
// application that is not tracking the mouse. Any other value means every
// press and release is reported to it. An empty string is a terminal that did
// not answer, read as not tracking, so DecideLinkPress swallows only a press
// it is sure about.
const MouseTrackingNone = "none"

// DecideLinkPress reports whether Gridwell alone owns a left-button press in
// a live shell, rather than the terminal.
//
// Gridwell owns a press over a hovered link while the application is tracking
// the mouse, because xterm both activates the link and reports the press, so
// an application with its own opener would open the url a second time in the
// host browser. A link opens inside Gridwell and nowhere else.
//
// Two presses stay the terminal's. With nothing tracking the mouse the press
// is xterm's selection start, which must keep working over a url, and a held
// modifier is the terminal's escape hatch from a tracking application.
//
// hoveredURL is the link xterm says the pointer is on, "" for none.
// mouseTracking is xterm's modes.mouseTrackingMode. modifier is whether any
// of alt, shift, control or meta is held.
func DecideLinkPress(hoveredURL, mouseTracking string, modifier bool) bool {
	if hoveredURL == "" || modifier {
		return false
	}
	return mouseTracking != "" && mouseTracking != MouseTrackingNone
}
