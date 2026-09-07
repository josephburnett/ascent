package store

import "github.com/josephburnett/gridwell/api/idshape"

// Identity minting lives in api/idshape: id shape is contract, not storage, so
// a third-party plugin and the host agree without importing each other. Only
// the store's call site is here.

// newUUID returns a fresh 128-bit id (system.plugin_uuid).
func newUUID() string { return idshape.NewUUID() }
