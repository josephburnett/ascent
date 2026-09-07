package namespace

// Minter is the one thing a namespace may offer beyond the wire method set:
// naming the canonical id a stored reference must hold. A plugin adapter
// (pluginhost.Adapter) answers a thing by what it is and accepts several id
// shapes for it, so without this a link could hold a different id than the
// listing answers for one document. The router asks before it stores. A
// namespace that does not implement Minter keeps the id it was given, which is
// what home does and what a mount of another node does, since the far node
// names its own things. It is deliberately not part of Namespace, because that
// interface is the gridwell.v1 service's method set in Go and this is not a
// wire verb.

import "context"

// Minter turns a local id into the canonical local id a stored reference must
// hold. An id that is already canonical answers itself.
type Minter interface {
	MintRef(ctx context.Context, localID string) (string, error)
}

// MintRef asks ns to canonicalize localID if it can, and otherwise leaves it
// alone. It is written once so that no caller has to know which namespaces
// derive ids.
func MintRef(ctx context.Context, ns Namespace, localID string) (string, error) {
	m, ok := ns.(Minter)
	if !ok {
		return localID, nil
	}
	return m.MintRef(ctx, localID)
}
