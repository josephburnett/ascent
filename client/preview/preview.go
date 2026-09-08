// Package preview holds the preview-image cache shared by URL and shell tiles.
// It keys a decoded Image by tile id and remembers which preview_blob_id it
// came from, so Get taking the caller's expected blob id invalidates a stale
// entry with no explicit invalidation signal. Decoding is JS-only, so it sits
// behind the Decoder interface and tests inject a synchronous fake.
package preview

import "sync"

// Image is the decoded handle the renderer draws. Truthy reports whether the
// browser resource is loaded, and Revoke releases the backing object URL.
type Image interface {
	Truthy() bool
	Revoke()
}

// Decoder turns raw JPEG bytes into an Image. The wasm decoder is
// asynchronous; the test fake resolves inside Decode so tests are
// deterministic.
type Decoder interface {
	Decode(bytes []byte, onReady func(Image), onError func())
}

// Cache invalidates an entry when the server-side preview blob id changes. One
// mutex protects the entry map, so every method is goroutine-safe.
type Cache struct {
	dec Decoder

	mu      sync.Mutex
	entries map[string]*entry
}

type entry struct {
	// blobID is the preview_blob_id decoded from, or wildcardBlobID.
	blobID int64
	// image is nil while a decode is pending.
	image Image
	// gen rises with every Put, so a decode whose onReady fires after a newer
	// Put superseded it is discarded.
	gen int64
	// empty is a settled miss. Without it every frame re-asks the server for
	// tiles that will never have a preview.
	empty bool
}

// wildcardBlobID marks bytes captured locally before the server blob id was
// known. Get matches it against any non-zero expected blob id.
const wildcardBlobID int64 = -1

// NewCache requires a non-nil dec.
func NewCache(dec Decoder) *Cache {
	return &Cache{
		dec:     dec,
		entries: map[string]*entry{},
	}
}

// Get hits when the entry's image is loaded and its blob id matches wantBlobID
// or is the wildcard. A wantBlobID of 0 means the server says the tile is
// blank, so an entry keyed to a real blob id misses; a wildcard entry hits,
// being a local capture parked ahead of the server.
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

// Put stores bytes belonging to a known server-side preview blob; locally
// captured bytes go through PutWildcard. onReady may be nil and fires once the
// entry is installed. A newer Put for the same tileID supersedes this one,
// discarding the late result silently.
func (c *Cache) Put(tileID string, blobID int64, bytes []byte, onReady func()) {
	c.put(tileID, blobID, bytes, onReady)
}

// PutEmpty records that the server answered with no preview. A completed fetch
// settles the cache either way, an unsettled empty result re-firing on every
// draw. A later Put, or a changed blob id, supersedes it.
func (c *Cache) PutEmpty(tileID string, blobID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[tileID]; ok && e.image != nil && e.image.Truthy() {
		return // a real image is never downgraded to a recorded miss
	}
	c.entries[tileID] = &entry{blobID: blobID, empty: true}
}

// KnownEmpty lets the caller skip the fetch instead of re-asking every frame.
func (c *Cache) KnownEmpty(tileID string, blobID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[tileID]
	return ok && e.empty && e.blobID == blobID
}

// PutWildcard serves the flows that hold JPEG bytes before the server blob id
// is known: the URL stream's frames and the shell freeze snapshot. The entry
// matches any non-zero wantBlobID until a specific Put supersedes it.
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
			// No image is installed. The entry's gen has already risen, which
			// discards any in-flight predecessor, and Get keeps returning the
			// prior image if there is one.
		},
	)
}

// Drop revokes the entry's image. It is idempotent and runs on tile delete.
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
