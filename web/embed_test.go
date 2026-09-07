package web

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
)

// The embedded pair must agree. gridwell.wasm.gz is served to every
// gzip-accepting browser in place of gridwell.wasm (internal/server's
// serveGzipSidecar), and the server's freshness guard cannot tell the two
// apart here: every embed.FS file carries the same zero modtime, so a
// mismatched pair is indistinguishable from a good one at serve time. It has
// to be caught before the embed. A gzip of a PREFIX of the wasm — what a
// `gzip` racing an in-progress `go build` writes — is a valid gzip stream that
// decompresses without error, so only a byte comparison against the raw file
// finds it. The browser's symptom is "WebAssembly.Module doesn't parse ...
// exceeds the module's remaining size" on a clean 200.

// sidecarMismatch returns nil when name+".gz" in fsys decompresses to exactly
// name, and an error describing the divergence otherwise. It streams: the raw
// wasm is tens of megabytes.
func sidecarMismatch(fsys fs.FS, name string) error {
	raw, err := fsys.Open(name)
	if err != nil {
		return err
	}
	defer raw.Close()
	gzf, err := fsys.Open(name + ".gz")
	if err != nil {
		return err
	}
	defer gzf.Close()
	zr, err := gzip.NewReader(gzf)
	if err != nil {
		return fmt.Errorf("%s.gz is not a gzip stream: %w", name, err)
	}
	defer zr.Close()

	const chunk = 1 << 16
	rawBuf := make([]byte, chunk)
	gzBuf := make([]byte, chunk)
	var at int64
	for {
		n, rawErr := io.ReadFull(raw, rawBuf)
		m, gzErr := io.ReadFull(zr, gzBuf)
		if n != m || !bytes.Equal(rawBuf[:n], gzBuf[:m]) {
			return fmt.Errorf("%s.gz does not decompress to %s: they diverge at byte %d (raw read %d, sidecar read %d)", name, name, at, n, m)
		}
		at += int64(n)
		rawDone := errors.Is(rawErr, io.EOF) || errors.Is(rawErr, io.ErrUnexpectedEOF)
		gzDone := errors.Is(gzErr, io.EOF) || errors.Is(gzErr, io.ErrUnexpectedEOF)
		if rawDone != gzDone {
			return fmt.Errorf("%s.gz does not decompress to %s: one stream ended at byte %d and the other did not", name, name, at)
		}
		if rawDone {
			return nil
		}
		if rawErr != nil {
			return rawErr
		}
		if gzErr != nil {
			return gzErr
		}
	}
}

// TestEmbeddedWasmSidecarMatchesRaw is the permanent owner of the pair: it
// runs on the real embed.FS, so a binary built from a raced or short sidecar
// fails `make check` instead of failing in the browser.
func TestEmbeddedWasmSidecarMatchesRaw(t *testing.T) {
	if err := sidecarMismatch(FS, "gridwell.wasm"); err != nil {
		t.Fatalf("embedded wasm sidecar is bad — rebuild with `make wasm` and do not embed a raced artifact: %v", err)
	}
}

// TestSidecarMismatchDetectsTruncation pins the check's own logic against the
// exact shape of the bug: a well-formed gzip of a prefix.
func TestSidecarMismatchDetectsTruncation(t *testing.T) {
	raw := bytes.Repeat([]byte("wasm bytes "), 20000)
	gzOf := func(b []byte) []byte {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(b); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	good := fstest.MapFS{
		"app.wasm":    {Data: raw},
		"app.wasm.gz": {Data: gzOf(raw)},
	}
	if err := sidecarMismatch(good, "app.wasm"); err != nil {
		t.Errorf("matching pair reported bad: %v", err)
	}

	// A gzip of a prefix: valid, decompresses cleanly, shorter.
	short := fstest.MapFS{
		"app.wasm":    {Data: raw},
		"app.wasm.gz": {Data: gzOf(raw[:len(raw)/3])},
	}
	if err := sidecarMismatch(short, "app.wasm"); err == nil {
		t.Error("a gzip of a prefix passed — this is exactly the raced sidecar")
	}

	// And divergence with no length difference, so the check is not just a
	// size comparison in disguise.
	altered := append([]byte(nil), raw...)
	altered[len(altered)/2] ^= 0xff
	differs := fstest.MapFS{
		"app.wasm":    {Data: raw},
		"app.wasm.gz": {Data: gzOf(altered)},
	}
	if err := sidecarMismatch(differs, "app.wasm"); err == nil {
		t.Error("a same-length but different sidecar passed")
	}
}
