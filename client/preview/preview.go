// Package preview holds the preview-image cache shared by URL tiles and shell
// tiles. Both store a JPEG in the server's blobs table under the tile's
// preview_blob_id, and the cache turns those bytes into a decoded Image keyed
// by tile id, remembering which blob id it came from. Get takes the caller's
// expected blob id, so a server-side preview change invalidates the stale entry
// on the next Get without any explicit invalidation signal.
//
// Decoding is JS-only, needing Blob, createObjectURL and new Image() in the
// browser, so it sits behind the Decoder interface. The wasm build ships a real
// decoder and unit tests inject a fake that resolves synchronously.
package preview

import "sync"

// Image is the decoded handle the renderer draws. Truthy reports whether the
// underlying browser resource is loaded, and Revoke releases the backing object
// URL. The wasm build wraps an HTMLImageElement and its createObjectURL; tests
// use a struct that records its revoked state.
type Image interface {
	Truthy() bool
	Revoke()
}

// Decoder turns raw JPEG bytes into an Image. onReady fires with the decoded
// image on success and onError fires on failure. The wasm decoder is
// asynchronous, and the test fake resolves synchronously inside Decode so tests
// are deterministic.
type Decoder interface {
	Decode(bytes []byte, onReady func(Image), onError func())
}

// Cache is a tile-id-keyed image cache that invalidates an entry when the
// server-side preview blob id changes. One mutex protects the entry map, so
// every method is safe to call from multiple goroutines.
type Cache struct {
	dec Decoder

	mu      sync.Mutex
	entries map[string]*entry
}

// entry holds one tile's cached preview state.
type entry struct {
	// blobID is the preview_blob_id this image was decoded from, or
	// wildcardBlobID for locally captured bytes whose server blob id is not
	// known yet.
	blobID int64
	// image is the decoded handle, or nil while a decode is pending.
	image Image
	// gen rises with every Put. A decode whose onReady fires after a newer Put
	// superseded it is discarded.
	gen int64
	// empty records a completed fetch that answered with no preview for
	// blobID, which is a settled miss. Without it every frame re-asks the
	// server for tiles that will never have a preview.
	empty bool
}

// wildcardBlobID marks an entry whose bytes were captured locally before the
// server-side blob id was known. Get treats it as a match for any non-zero
// expected blob id.
const wildcardBlobID int64 = -1

// NewCache returns a Cache backed by dec, which must be non-nil.
func NewCache(dec Decoder) *Cache {
	return &Cache{
		dec:     dec,
		entries: map[string]*entry{},
	}
}

// Get returns the cached image for tileID when an entry exists, its image is
// loaded, and its recorded blob id matches wantBlobID or is the wildcard.
//
// A wantBlobID of 0 means the tile has no server-side preview yet. An entry
// keyed to a real blob id misses then, because it is server state that may be
// stale and the server says the tile is blank. A wildcard entry hits, because
// it is a local capture parked ahead of the server, such as the first freeze of
// a url or shell tile whose PreviewBlobID stays 0 until the SetURLState or
// SetShellPreview echo lands.
func (c *Cache) Get(tileID string, wantBlobID int64) (Image, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[tileID]
	if !ok || e.image == nil || !e.image.Truthy() {
		return nil, false
	}
	if e.blobID != wildcardBlobID && (wantBlobID == 0 || e.blobID != wantBlobID) {
		return nil, false
	}
	return e.image, true
}

// Put decodes bytes and stores them under (tileID, blobID). Use it when the
// bytes belong to a known server-side preview blob, such as the result of
// GetTilePreview; for locally captured bytes whose blob id is not known yet,
// use PutWildcard.
//
// onReady fires once the decode completes and the entry is installed, and may
// be nil. A newer Put for the same tileID supersedes this one, discarding the
// late result without calling onReady.
func (c *Cache) Put(tileID string, blobID int64, bytes []byte, onReady func()) {
	c.put(tileID, blobID, bytes, onReady)
}

// PutEmpty records that the server answered with no preview for
// (tileID, blobID). A completed fetch settles the cache either way, and an
// unsettled empty result would re-fire the fetch on every draw. A later Put
// with real bytes, or a changed blob id, supersedes it.
func (c *Cache) PutEmpty(tileID string, blobID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[tileID]; ok && e.image != nil && e.image.Truthy() {
		return // a real image is never downgraded to a recorded miss
	}
	c.entries[tileID] = &entry{blobID: blobID, empty: true}
}

// KnownEmpty reports a recorded no-preview answer for (tileID, blobID), so the
// caller skips the fetch instead of re-asking every frame.
func (c *Cache) KnownEmpty(tileID string, blobID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[tileID]
	return ok && e.empty && e.blobID == blobID
}

// PutWildcard decodes bytes and stores them under tileID with the wildcard
// sentinel. It serves the flows that hold the JPEG bytes before the server-side
// blob id is known, the URL stream's WebSocket frames and the shell freeze
// snapshot. The entry matches any non-zero wantBlobID in Get until a specific
// Put supersedes it.
func (c *Cache) PutWildcard(tileID string, bytes []byte, onReady func()) {
	c.put(tileID, wildcardBlobID, bytes, onReady)
}

func (c *Cache) put(tileID string, blobID int64, bytes []byte, onReady func()) {
	if len(bytes) == 0 {
		return
	}
	c.mu.Lock()
	e, ok := c.entries[tileID]
	if !ok {
		e = &entry{}
		c.entries[tileID] = e
	}
	e.gen++
	gen := e.gen
	c.mu.Unlock()

	c.dec.Decode(bytes,
		func(img Image) {
			c.mu.Lock()
			cur, ok := c.entries[tileID]
			if !ok || cur.gen != gen {
				// Superseded or dropped before the decode completed.
				c.mu.Unlock()
				if img != nil {
					img.Revoke()
				}
				return
			}
			if cur.image != nil && cur.image.Truthy() {
				cur.image.Revoke()
			}
			cur.image = img
			cur.blobID = blobID
			c.mu.Unlock()
			if onReady != nil {
				onReady()
			}
		},
		func() {
			// The decode failed, so no image is installed. The entry's gen has
			// already risen, which also discards any in-flight predecessor for
			// the same tile, and Get keeps returning the prior image if there
			// is one.
		},
	)
}

// Drop removes the entry for tileID and revokes its image, if any. It is
// idempotent and runs when a tile is deleted.
func (c *Cache) Drop(tileID string) {
	c.mu.Lock()
	e, ok := c.entries[tileID]
	if ok {
		delete(c.entries, tileID)
	}
	c.mu.Unlock()
	if ok && e.image != nil && e.image.Truthy() {
		e.image.Revoke()
	}
}
