//go:build js && wasm

package main

import (
	gridwellv1 "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
	"math"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/josephburnett/gridwell/client/markdown"
	"github.com/josephburnett/gridwell/client/textedit"
)

// Rendered grid previews: a text tile whose stored text_mode is "rendered"
// previews as the rendered document, so how you leave a tile is how it
// presents from outside. markdown.RenderHTML stays the one renderer, and this
// rasterizes its output through an SVG foreignObject image. Rasterization is
// async, so raw source paints until the image decodes.

// renderedPreviewMaxH caps the rasterized document height in CSS px. Beyond
// it a preview falls back to raw source: previews are a glance, not a
// reader.
const renderedPreviewMaxH = 4000.0

// renderedPreviewBucket quantizes the layout width so continuous grid zoom
// re-rasterizes at steps, not per frame.
const renderedPreviewBucket = 64.0

// renderedPreview is one tile's cached raster.
type renderedPreview struct {
	key     string
	img     js.Value
	url     string
	rasterW float64
	ready   bool
	failed  bool
}

// renderedPreviewFor returns the raster for tile n at roughly logical width
// contentW, kicking an async rasterization on a miss. ok stays false until
// the image decodes, so the caller paints raw source. The cache is keyed per
// (tile, width bucket): two consumers at different widths would otherwise
// replace one entry every frame, each revoking the other's loading blob
// URL.
func (a *App) renderedPreviewFor(n *gridwellv1.Tile, contentW float64) (*renderedPreview, bool) {
	bucket := math.Max(renderedPreviewBucket,
		math.Round(contentW/renderedPreviewBucket)*renderedPreviewBucket)
	isOrg := markdown.IsOrg(n.AltText)
	mapKey := n.Id + "\x00" + strconv.FormatFloat(bucket, 'f', 0, 64)
	key := n.Id + "\x00" + strconv.FormatInt(n.Version, 10) + "\x00" +
		strconv.FormatFloat(bucket, 'f', 0, 64) + "\x00" + strconv.FormatBool(isOrg)
	if e, ok := a.views.renderedPrev[mapKey]; ok && e.key == key {
		return e, e.ready && !e.failed
	}
	body, ok := a.tileBody(n)
	if !ok {
		return nil, false // blob fetch in flight; the raw path warms it too
	}
	// Replace a stale same-bucket entry and sweep other buckets whose version
	// moved on; they re-rasterize on next use.
	if old, ok := a.views.renderedPrev[mapKey]; ok && old.url != "" {
		js.Global().Get("URL").Call("revokeObjectURL", old.url)
	}
	stalePrefix := n.Id + "\x00"
	for mk, old := range a.views.renderedPrev {
		if mk != mapKey && strings.HasPrefix(mk, stalePrefix) && old.key != "" &&
			!strings.HasPrefix(old.key, n.Id+"\x00"+strconv.FormatInt(n.Version, 10)+"\x00") {
			if old.url != "" {
				js.Global().Get("URL").Call("revokeObjectURL", old.url)
			}
			delete(a.views.renderedPrev, mk)
		}
	}
	e := &renderedPreview{key: key, rasterW: bucket}
	a.views.renderedPrev[mapKey] = e

	// The SVG foreignObject is an XML context, so serialize through the DOM
	// to make goldmark's HTML5 output well-formed.
	div := a.doc.Call("createElement", "div")
	div.Set("innerHTML", textedit.PresentationHTML(n, body))
	xhtml := js.Global().Get("XMLSerializer").New().Call("serializeToString", div).String()
	svg := markdown.PreviewSVG(xhtml, bucket, renderedPreviewMaxH, colorFileInnerBg)

	blob := js.Global().Get("Blob").New(
		js.ValueOf([]any{svg}), js.ValueOf(map[string]any{"type": "image/svg+xml"}))
	e.url = js.Global().Get("URL").Call("createObjectURL", blob).String()
	img := js.Global().Get("Image").New()
	var onload, onerror js.Func
	release := func() { onload.Release(); onerror.Release() }
	onload = js.FuncOf(func(js.Value, []js.Value) any {
		e.ready = true
		release()
		a.draw()
		return nil
	})
	onerror = js.FuncOf(func(js.Value, []js.Value) any {
		e.failed = true // raw source stays the preview; never retry-loop
		release()
		return nil
	})
	img.Set("onload", onload)
	img.Set("onerror", onerror)
	img.Set("src", e.url)
	e.img = img
	return e, false
}

// drawRenderedPreview windows the tile's raster at the preview frame's
// scroll, reporting whether it drew. False means the caller paints the raw
// fallback.
func (a *App) drawRenderedPreview(n *gridwellv1.Tile, frame markdown.PreviewFrame,
	x, y, w, h, topInset float64) bool {
	e, ok := a.renderedPreviewFor(n, frame.ContentW)
	if !ok {
		return false
	}
	s := w / e.rasterW
	if s <= 0 {
		return false
	}
	sy := frame.ScrollY
	sh := (h - topInset) / s
	if sy < 0 || sy >= renderedPreviewMaxH {
		return false
	}
	if sy+sh > renderedPreviewMaxH {
		sh = renderedPreviewMaxH - sy
	}
	a.cctx.Call("drawImage", e.img, 0, sy, e.rasterW, sh,
		x, y+topInset, w, sh*s)
	return true
}

// dropRenderedPreview releases a removed tile's entries, revoking their blob
// URLs. Fired from the TileRemoved arm beside urlPreview.Drop, so the two
// preview caches age out together and deleting text tiles leaks nothing.
func (a *App) dropRenderedPreview(tileID string) {
	prefix := tileID + "\x00"
	for mk, e := range a.views.renderedPrev {
		if !strings.HasPrefix(mk, prefix) {
			continue
		}
		if e.url != "" {
			js.Global().Get("URL").Call("revokeObjectURL", e.url)
		}
		delete(a.views.renderedPrev, mk)
	}
}
