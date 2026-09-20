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

type FlowClient struct{ RPC pb.FlowManagementClient }

func NewFlowClient(conn grpc.ClientConnInterface) *FlowClient {
	return &FlowClient{RPC: pb.NewFlowManagementClient(conn)}
}
func (c *FlowClient) ReadFlow(ctx context.Context, scope app.DraftScope, kind, id string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	query := &pb.FlowQuery{Scope: draftScope(scope)}
	var raw *pb.SolutionResponse
	var err error
	options := []grpc.CallOption{grpc.MaxCallRecvMsgSize(1024*1024 + 1024)}
	switch kind {
	case "solution":
		query.SolutionId = id
		raw, err = c.RPC.GetSolution(ctx, query, options...)
	case "publication":
		query.PublicationId = id
		raw, err = c.RPC.GetPublication(ctx, query, options...)
	default:
		return nil, app.ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	result, err := solutionPayload(raw, "qs-ai-flow/v1", "")
	if err != nil {
		return nil, err
	}
	var identity struct {
		ID     string `json:"source_id"`
		Kind   string `json:"source_kind"`
		Schema string `json:"schema_version"`
	}
	if json.Unmarshal(result, &identity) != nil || identity.ID != id || identity.Kind != kind || identity.Schema != "qs-ai-flow/v1" {
		return nil, status.Error(codes.Unavailable, "Flow evidence identity mismatch")
	}
	return result, nil
}
