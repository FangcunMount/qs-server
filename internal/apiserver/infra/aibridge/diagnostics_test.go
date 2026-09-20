package aibridge

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestCorrelationPreservesMetadataAndDoesNotRetry(t *testing.T) {
	for _, input := range []string{"75d9b24b-2864-4318-b015-bc6e5df9f419", "bad id!"} {
		ctx := middleware.WithRequestID(context.Background(), input)
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "test-only")
		calls := 0
		expected := errors.New("sentinel")
		err := correlateRPC(ctx, "/service/Call", nil, nil, nil, func(c context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			calls++
			md, _ := metadata.FromOutgoingContext(c)
			ids := md.Get(correlationMetadata)
			if len(ids) != 1 {
				t.Fatal("missing correlation")
			}
			if _, e := uuid.Parse(ids[0]); e != nil {
				t.Fatal(e)
			}
			if input != "bad id!" && ids[0] != input {
				t.Fatal("changed valid correlation")
			}
			if md.Get("authorization")[0] != "test-only" {
				t.Fatal("lost metadata")
			}
			return expected
		})
		if err != expected || calls != 1 {
			t.Fatal("changed call semantics")
		}
	}
}

func TestNormalizeCorrelationRejectsInvalid(t *testing.T) {
	if got := NormalizeCorrelationID("ok-id_1"); got != "ok-id_1" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeCorrelationID("bad id!"); got == "bad id!" {
		t.Fatal("accepted invalid")
	}
}
