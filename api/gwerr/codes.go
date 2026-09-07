package gwerr

import (
	"connectrpc.com/connect"
	"google.golang.org/grpc/codes"
)

// codePairs is the one gRPC-to-Connect status-code table. Gridwell answers
// in gRPC status codes everywhere and one hop translates, the Connect codec
// on the way to a browser (server.asConnectError). The table is total and
// injective over the gRPC enum (TestCodeTableIsTotal), so a missing code
// fails a test here instead of a user's write.
var codePairs = []struct {
	G codes.Code
	C connect.Code
}{
	{codes.Canceled, connect.CodeCanceled},
	{codes.Unknown, connect.CodeUnknown},
	{codes.InvalidArgument, connect.CodeInvalidArgument},
	{codes.DeadlineExceeded, connect.CodeDeadlineExceeded},
	{codes.NotFound, connect.CodeNotFound},
	{codes.AlreadyExists, connect.CodeAlreadyExists},
	{codes.PermissionDenied, connect.CodePermissionDenied},
	{codes.ResourceExhausted, connect.CodeResourceExhausted},
	{codes.FailedPrecondition, connect.CodeFailedPrecondition},
	{codes.Aborted, connect.CodeAborted},
	{codes.OutOfRange, connect.CodeOutOfRange},
	{codes.Unimplemented, connect.CodeUnimplemented},
	{codes.Internal, connect.CodeInternal},
	{codes.Unavailable, connect.CodeUnavailable},
	{codes.DataLoss, connect.CodeDataLoss},
	{codes.Unauthenticated, connect.CodeUnauthenticated},
}

// ConnectCode maps a gRPC status code to its Connect twin. codes.OK has no
// Connect error code and maps to Internal, as does any code off the table.
func ConnectCode(c codes.Code) connect.Code {
	for _, p := range codePairs {
		if p.G == c {
			return p.C
		}
	}
	return connect.CodeInternal
}
