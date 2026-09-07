// Package gwerr is the contract's error vocabulary: the sentinel errors a
// store or plugin answers with, and the sentinel-to-class table every
// transport maps from. It lives in the api module because the host maps the
// classes and a third-party plugin answers with the same sentinels, and
// neither may import the other.
package gwerr

import (
	"errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Sentinel errors. A caller uses errors.Is and a plugin may return one
// wrapped, so every transport classifies it the same way.
var (
	ErrNotFound        = errors.New("not found")
	ErrOverlap         = errors.New("footprint overlaps an existing tile")
	ErrInvalidPath     = errors.New("descent path is invalid")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotURLTile      = errors.New("not a URL tile")
	ErrNotTextTile     = errors.New("not a text tile")
	ErrNotWellTile     = errors.New("not a well tile")
	ErrNotShellTile    = errors.New("not a shell tile")
	ErrNotPaneTile     = errors.New("not a pane tile")
	ErrVersionConflict = errors.New("version mismatch")
	// ErrSchemaDivergence is a deployment problem, so its class is
	// ClassInternal.
	ErrSchemaDivergence = errors.New("database schema diverges from this binary's schema")
)

// ErrorClass is the transport-neutral category of a sentinel. Every
// transport maps from the one table below, so a new sentinel cannot degrade
// to Internal on one transport and not another.
type ErrorClass int

const (
	ClassInternal ErrorClass = iota
	ClassNotFound
	ClassInvalidArgument
	ClassConflict
)

// sentinelClasses is total over the exported Err* sentinels, the
// ClassInternal ones included. A sentinel missing here fails
// TestEverySentinelIsClassified.
var sentinelClasses = []struct {
	Err   error
	Class ErrorClass
}{
	{ErrNotFound, ClassNotFound},
	{ErrInvalidArgument, ClassInvalidArgument},
	{ErrInvalidPath, ClassInvalidArgument},
	{ErrNotURLTile, ClassInvalidArgument},
	{ErrNotTextTile, ClassInvalidArgument},
	{ErrNotWellTile, ClassInvalidArgument},
	{ErrNotShellTile, ClassInvalidArgument},
	{ErrNotPaneTile, ClassInvalidArgument},
	{ErrOverlap, ClassConflict},
	{ErrVersionConflict, ClassConflict},
	{ErrSchemaDivergence, ClassInternal},
}

// ClassifyError returns the class of a sentinel, wrapped or not. nil and
// any other error are ClassInternal, so a caller that must tell nil apart
// checks it first.
func ClassifyError(err error) ErrorClass {
	for _, s := range sentinelClasses {
		if errors.Is(err, s.Err) {
			return s.Class
		}
	}
	return ClassInternal
}

// IsTransport reports that the far side of a gRPC hop never spoke. Every
// server-side hop that degrades to a remembered answer keys on this and
// nothing else, so a coded answer such as NotFound passes through verbatim
// and is never served from a cache. clientsync.Of is the Connect-wire twin
// on the client, pinned to the same three codes.
func IsTransport(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
		return true
	}
	return false
}
