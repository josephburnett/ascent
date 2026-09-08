// Package clientsync holds the post-RPC policy the wasm client applies after
// a mutation returns: what the outcome was (Of) and what to do about it (one
// React table per mutation family). Local state may be dropped only on a
// server verdict; a transport failure keeps it and retries.
package clientsync

import (
	"context"
	"errors"

	"connectrpc.com/connect"
)

// Outcome is what an RPC's result meant.
type Outcome int

const (
	OutcomeOK Outcome = iota
	// OutcomeConflict is a version or overlap race; the local claim lost.
	OutcomeConflict
	// OutcomeRejected is the server saying no; the local attempt is wrong.
	OutcomeRejected
	// OutcomeTransport is the server never speaking. The local state is
	// still the only truth the user has, so the caller keeps it.
	OutcomeTransport
)

// Of classifies an RPC error. A non-connect error comes from below the
// protocol, so it is Transport too; every other coded error is a server that
// answered. A context deadline or cancellation is checked first and by
// identity, because the bound is inflight.Deadline, the client's own timer,
// and reading its expiry as a verdict would drop the user's bytes.
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

// Reaction is what a mutation's outcome calls for. Success is the zero value.
type Reaction struct {
	// Refetch is never set on Transport, where against a flapping link it
	// could succeed and revert a patch whose write never landed.
	Refetch bool
	// Log asks the caller to surface the failure through errsurface.
	Log bool
	// DropLocal permits reconciling away the local copy. It is true only on
	// a server verdict; false means the caller parks the state for a retry.
	DropLocal bool
	// Retry leaves the value queued for the reconnect drain, exactly on
	// Transport.
	Retry bool
}

// React is the policy for a mutation that wrote no local state ahead of the
// RPC, a create, move or delete. Transport surfaces but sets no Retry,
// because there is no ledger behind these ops to retry from.
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
// cache back, or it stays ahead of the server. Transport keeps the patch,
// which is the value the retry will land, and refetches nothing.
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
// version. On Transport the entry stays dirty, because it is the only copy of
// the user's unsaved words. A conflict is surfaced here where the other
// tables leave it silent: someone else changed these bytes and the words on
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

// IsUnimplemented reports a plugin's answer that it does not serve this call.
// It is a capability, never a failure to surface.
func IsUnimplemented(err error) bool {
	var ce *connect.Error
	return errors.As(err, &ce) && ce.Code() == connect.CodeUnimplemented
}
