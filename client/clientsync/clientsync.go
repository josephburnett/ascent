// Package clientsync holds the post-RPC policy the wasm client applies after
// a mutation returns: what the outcome was (Of) and what to do about it (one
// React table per mutation family). The decisions are data, so they are
// table-tested without a browser, a transport, or App state.
//
// The rule they enforce is that local state may be dropped only on a server
// verdict. A transport failure keeps the state and retries; only OutcomeOK,
// OutcomeConflict and OutcomeRejected may reconcile.
package clientsync

import (
	"context"
	"errors"

	"connectrpc.com/connect"
)

// Outcome is what an RPC's result meant. OutcomeTransport means the server
// never spoke, so no local state may be reconciled away on its strength.
type Outcome int

const (
	// OutcomeOK means the mutation landed.
	OutcomeOK Outcome = iota
	// OutcomeConflict is a version or overlap race. The local claim lost,
	// so the caller refetches.
	OutcomeConflict
	// OutcomeRejected means the server said no. The local attempt is wrong
	// and reconciles.
	OutcomeRejected
	// OutcomeTransport means the server never spoke. The local state is
	// still the only truth the user has, so the caller keeps it and
	// retries when the link returns.
	OutcomeTransport
)

// Of classifies an RPC error. TestOfPinsWireCodes pins the transport set to
// the three connect codes. A non-connect error comes from below the protocol,
// so it is Transport too, and every other coded error is a server that
// answered.
//
// A context deadline or cancellation is checked first and by identity, not by
// wire code. The bound on a client RPC is inflight.Deadline, the client's
// own, so its expiry means the server never spoke whatever code the transport
// dressed it in. Reading that as a verdict would drop the user's bytes on the
// client's own timer.
func Of(err error) Outcome {
	if err == nil {
		return OutcomeOK
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return OutcomeTransport
	}
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return OutcomeTransport
	}
	switch ce.Code() {
	case connect.CodeFailedPrecondition:
		return OutcomeConflict
	case connect.CodeUnavailable, connect.CodeDeadlineExceeded, connect.CodeCanceled:
		return OutcomeTransport
	}
	return OutcomeRejected
}

// Reaction is what a mutation's outcome calls for. Success is the zero
// value.
type Reaction struct {
	// Refetch asks the caller to re-fetch the affected grid. It is never
	// set on Transport, where against a flapping link it could succeed and
	// revert an optimistic patch whose write never landed.
	Refetch bool
	// Log asks the caller to surface the failure through errsurface.
	Log bool
	// DropLocal permits reconciling away the caller's local copy. It is
	// true only on a server verdict; false on a failure means the caller
	// keeps the state parked so a retry can land it.
	DropLocal bool
	// Retry asks the caller to leave the value queued for the reconnect
	// drain. It is set exactly on Transport.
	Retry bool
}

// React is the policy for a mutation that wrote no local state ahead of the
// RPC, such as a create, move or delete, where snapping the ghost back is the
// reconcile. Transport surfaces but sets no Retry, because there is no ledger
// behind these ops to retry from.
func React(o Outcome) Reaction {
	switch o {
	case OutcomeConflict:
		return Reaction{Refetch: true}
	case OutcomeRejected, OutcomeTransport:
		return Reaction{Log: true}
	}
	return Reaction{}
}

// ReactOptimistic is the policy for a mutation whose caller patched the local
// cache before the RPC, such as a framing write. Any server verdict rolls the
// cache back, Rejected included, or the cache stays ahead of the server.
// Transport keeps the patch, which is the value the retry will land, and
// refetches nothing (see Reaction.Refetch).
func ReactOptimistic(o Outcome) Reaction {
	switch o {
	case OutcomeConflict:
		return Reaction{Refetch: true, DropLocal: true}
	case OutcomeRejected:
		return Reaction{Refetch: true, Log: true, DropLocal: true}
	case OutcomeTransport:
		return Reaction{Log: true, Retry: true}
	}
	return Reaction{}
}

// ReactSave is the policy for a content save, the one write that claims a
// version. On a verdict the unsaved bytes reconcile away, so the screen shows
// what the server holds. On Transport the entry stays dirty, because it is
// the only copy of the user's unsaved words.
//
// A conflict is surfaced here where the other tables leave it silent, because
// a save conflict means someone else changed these bytes and the words on
// screen are about to be replaced by theirs.
func ReactSave(o Outcome) Reaction {
	switch o {
	case OutcomeConflict:
		return Reaction{Refetch: true, Log: true, DropLocal: true}
	case OutcomeRejected:
		return Reaction{Refetch: true, Log: true, DropLocal: true}
	case OutcomeTransport:
		return Reaction{Log: true, Retry: true}
	}
	return Reaction{}
}

// IsUnimplemented reports a plugin's answer that it does not serve this
// call. It is a capability property and never a failure to surface. It lives
// here so every wire-code judgment sits in one tested place.
func IsUnimplemented(err error) bool {
	var ce *connect.Error
	return errors.As(err, &ce) && ce.Code() == connect.CodeUnimplemented
}
