package store

import "github.com/josephburnett/gridwell/api/gwerr"

// The sentinel-to-class table lives in api/gwerr: the error vocabulary is
// contract. These aliases keep the store-side names.

type ErrorClass = gwerr.ErrorClass

const (
	ClassInternal        = gwerr.ClassInternal
	ClassNotFound        = gwerr.ClassNotFound
	ClassInvalidArgument = gwerr.ClassInvalidArgument
	ClassConflict        = gwerr.ClassConflict
)

// ClassifyError is gwerr.ClassifyError.
func ClassifyError(err error) ErrorClass { return gwerr.ClassifyError(err) }
