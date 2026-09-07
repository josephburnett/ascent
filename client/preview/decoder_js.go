//go:build js && wasm

package preview

import "syscall/js"

// JSDecoder is the production Decoder. It feeds bytes through the browser's
// Blob, URL.createObjectURL and new Image() chain, so the browser does the
// decode and onReady fires on the JS event loop once the load event resolves.
type JSDecoder struct{}

// NewJSDecoder returns a Decoder suitable for the wasm build.
func NewJSDecoder() JSDecoder { return JSDecoder{} }

// Decode implements Decoder. The decoded handle is a *JSImage whose Val gives
// the renderer the HTMLImageElement for canvas.drawImage. Each Decode allocates
// an onload and an onerror js.Func, and both are released once either fires.
func (JSDecoder) Decode(bytes []byte, onReady func(Image), onError func()) {
	if len(bytes) == 0 {
		if onError != nil {
			onError()
		}
		return
	}
	u8 := js.Global().Get("Uint8Array").New(len(bytes))
	js.CopyBytesToJS(u8, bytes)
	blobOpts := js.Global().Get("Object").New()
	blobOpts.Set("type", "image/jpeg")
	parts := js.Global().Get("Array").New()
	parts.Call("push", u8)
	blob := js.Global().Get("Blob").New(parts, blobOpts)
	objectURL := js.Global().Get("URL").Call("createObjectURL", blob).String()

	img := js.Global().Get("Image").New()
	var onload, onerr js.Func
	onload = js.FuncOf(func(js.Value, []js.Value) any {
		onload.Release()
		onerr.Release()
		if onReady != nil {
			onReady(&JSImage{val: img, objectURL: objectURL})
		} else {
			// With no consumer, revoke here so the object URL does not leak.
			js.Global().Get("URL").Call("revokeObjectURL", objectURL)
		}
		return nil
	})
	onerr = js.FuncOf(func(js.Value, []js.Value) any {
		onload.Release()
		onerr.Release()
		js.Global().Get("URL").Call("revokeObjectURL", objectURL)
		if onError != nil {
			onError()
		}
		return nil
	})
	img.Set("onload", onload)
	img.Set("onerror", onerr)
	img.Set("src", objectURL)
}

// JSImage wraps an HTMLImageElement and the createObjectURL it was loaded from.
// The renderer reaches the raw js.Value through Val, and the cache calls Revoke
// when the entry is replaced or dropped.
type JSImage struct {
	val       js.Value
	objectURL string
	revoked   bool
}

// Val returns the underlying HTMLImageElement for canvas.drawImage.
func (i *JSImage) Val() js.Value { return i.val }

// Truthy reports whether the element is still usable. After Revoke the object
// URL is gone and the image would fail to paint, so it reports false.
func (i *JSImage) Truthy() bool {
	if i == nil || i.revoked {
		return false
	}
	return i.val.Truthy()
}

// Revoke releases the createObjectURL. It is idempotent, and the cache calls
// it when a newer Put supersedes the entry or Drop removes it.
func (i *JSImage) Revoke() {
	if i == nil || i.revoked {
		return
	}
	i.revoked = true
	js.Global().Get("URL").Call("revokeObjectURL", i.objectURL)
}
