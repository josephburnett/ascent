package namespace

// Minter is the one thing a namespace may offer beyond the wire method set:
// naming the canonical id a stored reference must hold. A plugin adapter
// accepts several id shapes for one thing, so without this a link could hold a
// different id than the listing answers. The router asks before it stores; a
// namespace without Minter keeps the id it was given, as home and a mount of
// another node do. It is not part of Namespace, which is the gridwell.v1
// method set and this is not a wire verb.

import "context"

// Minter turns a local id into the canonical local id a stored reference must
// hold. An id that is already canonical answers itself.
type Minter interface {
	MintRef(ctx context.Context, localID string) (string, error)
}

// MintRef is written once, so no caller has to know which namespaces derive
// ids.
func MintRef(ctx context.Context, ns Namespace, localID string) (string, error) {
	m, ok := ns.(Minter)
	if !ok {
		return localID, nil
	}
	return m.MintRef(ctx, localID)
}
