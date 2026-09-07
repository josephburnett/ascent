package namespace

// Minter is the one thing a namespace may offer beyond the wire method set:
// naming the canonical id a stored reference must hold.
//
// A namespace whose ids are all rows — home — needs nothing here. A namespace
// that answers a thing by what it IS, which is pluginhost.Adapter, needs a way
// to say which of the shapes it accepts is the one PUBLIC name, so a reference
// at rest holds the same id the listing answers and a document reached through
// a link is the same document reached in place. The router asks before it
// stores, and a namespace that does not implement Minter simply keeps the id it
// was given, which is what home does and what a mount of another node does —
// the far node names its own things, and there is no wire verb to ask it to.
//
// It is deliberately NOT part of Namespace: that interface is the gridwell.v1
// service's method set in Go, and this is not a wire verb.

import "context"

// Minter turns a local id into the canonical local id a stored reference must
// hold. An id that is already canonical answers itself.
type Minter interface {
	MintRef(ctx context.Context, localID string) (string, error)
}

// MintRef asks ns to canonicalize localID if it can, and otherwise leaves it
// alone. It is the one call site's worth of type assertion, written once so
// the router never has to know which namespaces derive ids.
func MintRef(ctx context.Context, ns Namespace, localID string) (string, error) {
	m, ok := ns.(Minter)
	if !ok {
		return localID, nil
	}
	return m.MintRef(ctx, localID)
}
