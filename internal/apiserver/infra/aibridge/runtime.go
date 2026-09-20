package aibridge

import (
	"context"
	"encoding/json"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

type RuntimeClient struct{ RPC pb.RuntimeManagementClient }

func NewRuntimeClient(conn grpc.ClientConnInterface) *RuntimeClient {
	return &RuntimeClient{RPC: pb.NewRuntimeManagementClient(conn)}
}
func (c *RuntimeClient) ReadRuntime(ctx context.Context, scope app.DraftScope, ids []string, detail bool) (json.RawMessage, error) {
	if len(ids) < 1 || len(ids) > 50 || (detail && len(ids) != 1) {
		return nil, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	q := &pb.RuntimeQuery{Scope: draftScope(scope), SessionIds: ids}
	var result *pb.RuntimeResponse
	var err error
	schema := "qs-ai-runtime-batch/v1"
	opts := []grpc.CallOption{grpc.MaxCallRecvMsgSize(262144 + 1024)}
	if detail {
		schema = "qs-ai-runtime-detail/v1"
		result, err = c.RPC.Get(ctx, q, opts...)
	} else {
		result, err = c.RPC.BatchGet(ctx, q, opts...)
	}
	if err != nil {
		if status.Code(err) == codes.PermissionDenied || status.Code(err) == codes.Unauthenticated {
			return nil, app.ErrGovernanceDenied
		}
		return nil, err
	}
	if result == nil || result.SchemaVersion != schema || len(result.DataJson) > 262144 || !json.Valid([]byte(result.DataJson)) {
		return nil, status.Error(codes.Unavailable, "Invalid runtime evidence")
	}
	return json.RawMessage(result.DataJson), nil
}

func (c *RuntimeClient) ReadRuntimeHealth(ctx context.Context, scope app.DraftScope) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	result, err := c.RPC.Health(ctx, &pb.RuntimeQuery{Scope: draftScope(scope)}, grpc.MaxCallRecvMsgSize(16384))
	if err != nil {
		if status.Code(err) == codes.PermissionDenied || status.Code(err) == codes.Unauthenticated {
			return nil, app.ErrGovernanceDenied
		}
		return nil, err
	}
	if result == nil || result.SchemaVersion != "qs-ai-runtime-health/v1" || !json.Valid([]byte(result.DataJson)) {
		return nil, app.ErrManagementUnavailable
	}
	return json.RawMessage(result.DataJson), nil
}
