package aibridge

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/pkg/aidiagnostics"

	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const correlationMetadata = "x-correlation-id"

// correlateRPC only adds diagnostic metadata. It never changes authorization or retries.
func correlateRPC(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	id := NormalizeCorrelationID(middleware.RequestIDFromStandardContext(ctx))
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(correlationMetadata, id)
	ctx = metadata.NewOutgoingContext(ctx, md)
	started := time.Now()
	err := invoke(ctx, method, req, reply, cc, opts...)
	requestID := ""
	if request, ok := req.(interface{ GetRequestId() string }); ok {
		requestID = request.GetRequestId()
	}
	aidiagnostics.Completed(ctx, method, started, err, requestID, "")
	return err
}

// NormalizeCorrelationID validates non-sensitive transport correlation only.
func NormalizeCorrelationID(value string) string {
	return aidiagnostics.NormalizeCorrelationID(value)
}
