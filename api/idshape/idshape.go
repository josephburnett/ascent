// Package idshape owns the identity shapes of the Gridwell contract: the
// short plugin and node id mint, the 128-bit mint behind
// system.plugin_uuid, and the rules a namespace segment must satisfy. It
// sits in the api module because a third-party plugin minting a connection
// namespace and the host validating a hand-edited server.yaml must agree
// without either importing the other.
package idshape

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// NewUUID returns a fresh random 128-bit id as 32 hex characters. No caller
// parses one, so it carries no 8-4-4-4-12 grouping.
func NewUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// shortIDLen gives about 35.7 bits, so a personal node reaches even odds of
// a collision only around 300k ids, and the id stays readable in a URL.
const shortIDLen = 7

// NewShortID returns a fresh plugin, node or namespace identity: shortIDLen
// characters of lowercase base36 starting with a letter. Three properties
// are required of it. Lowercase, because the id names a directory
// (~/.gridwell/db/<id>) on case-insensitive filesystems and a tmux socket.
// No '/', so the qualified-id codec (rpc.SplitID) keeps its delimiter. A
// leading letter, so the id never parses as an integer, which is how a URL
// path tells a namespace segment from a tile id.
//
// The 32-hex shape stays valid forever and every consumer accepts both. An
// id is immutable once minted, because it lives in other plugins' stored
// references, session partitions and socket names.
func NewShortID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	const alnum = "0123456789abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, shortIDLen)
	b[0] = letters[randBelow(len(letters))]
	for i := 1; i < shortIDLen; i++ {
		b[i] = alnum[randBelow(len(alnum))]
	}
	return string(b)
}

// ValidateSegment checks an id used as a namespace segment: a plugin id, a
// node id, a connection namespace. It must not be empty, must not contain
// the qualified-id delimiter '/', and must not take either tile shape, which
// a URL path could not tell from a tile: purely numeric, or leading with
// '~'. what names the id in the error.
//
// The tile shapes are decided by rpc.ShapeOf. This package cannot import
// api/rpc because rpc's tests import this one, so a test there pins the two
// spellings to each other.
//
// The empty segment is refused here rather than by each caller: a nameless
// declaration would occupy it and "<node>//12" would peel to nothing. An id
// that may legitimately be absent, one Mint fills in, is checked for
// presence by its caller before it gets here.
func ValidateSegment(what, id string) error {
	if id == "" {
		return fmt.Errorf("%s is empty — a namespace segment must name something", what)
	}
	if strings.Contains(id, "/") {
		return fmt.Errorf("%s %q must not contain '/'", what, id)
	}
	if _, err := strconv.ParseInt(id, 10, 64); err == nil {
		return fmt.Errorf("%s %q must not be purely numeric (indistinguishable from a tile id)", what, id)
	}
	if strings.HasPrefix(id, "~") {
		return fmt.Errorf("%s %q must not begin with '~' (indistinguishable from a key-form tile id)", what, id)
	}
	return nil
}

// randBelow draws from crypto/rand without modulo bias, so the id space
// keeps its full entropy.
func randBelow(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic("idshape: crypto/rand failed: " + err.Error())
	}
	return int(v.Int64())
}
