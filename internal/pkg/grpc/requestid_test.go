package grpc

import (
	"context"
	"testing"

	basegrpc "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestPropagatingRequestIDInterceptorUsesIncomingMetadata(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-request-id", "request-existing"))
	_, err := propagatingRequestIDInterceptor()(ctx, nil, &grpcpkg.UnaryServerInfo{}, func(callCtx context.Context, _ interface{}) (interface{}, error) {
		if got := basegrpc.RequestIDFromContext(callCtx); got != "request-existing" {
			t.Fatalf("request ID = %q", got)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
