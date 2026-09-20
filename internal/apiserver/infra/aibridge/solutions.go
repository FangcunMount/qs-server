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

type SolutionClient struct{ RPC pb.SolutionManagementClient }

func NewSolutionClient(conn grpc.ClientConnInterface) *SolutionClient {
	return &SolutionClient{RPC: pb.NewSolutionManagementClient(conn)}
}
func solutionPayload(raw *pb.SolutionResponse, schema, id string) (json.RawMessage, error) {
	if raw == nil || raw.SchemaVersion != schema || len(raw.DataJson) > 1024*1024 || !json.Valid([]byte(raw.DataJson)) {
		return nil, status.Error(codes.Unavailable, "Solution receipt requires reconciliation")
	}
	if schema == "qs-ai-solution/v1" {
		var state struct {
			ID       string `json:"solution_id"`
			Revision int64  `json:"revision"`
			Schema   string `json:"schema_version"`
		}
		if json.Unmarshal([]byte(raw.DataJson), &state) != nil || !app.ValidPublicationID(state.ID) || state.Revision < 1 || state.Schema != schema || (id != "" && state.ID != id) {
			return nil, status.Error(codes.Unavailable, "Solution receipt requires reconciliation")
		}
	}
	return json.RawMessage(raw.DataJson), nil
}
func (c *SolutionClient) ReadSolution(ctx context.Context, scope app.DraftScope, operation, id, cursor string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	query := &pb.SolutionQuery{Scope: draftScope(scope), SolutionId: id, Cursor: cursor}
	opts := []grpc.CallOption{grpc.MaxCallRecvMsgSize(1024*1024 + 1024)}
	var raw *pb.SolutionResponse
	var err error
	schema := "qs-ai-solution/v1"
	switch operation {
	case "get":
		raw, err = c.RPC.Get(ctx, query, opts...)
	case "receipt":
		query.SolutionId = ""
		query.CommandId = id
		id = ""
		raw, err = c.RPC.GetReceipt(ctx, query, opts...)
	case "list":
		schema = "qs-ai-solutions/v1"
		raw, err = c.RPC.List(ctx, query, opts...)
	case "models":
		schema = "qs-ai-solution-models/v1"
		raw, err = c.RPC.GetModels(ctx, query, opts...)
	default:
		return nil, app.ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	return solutionPayload(raw, schema, id)
}
func (c *SolutionClient) WriteSolution(ctx context.Context, scope app.DraftScope, operation, id string, body json.RawMessage) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	command := &pb.SolutionWrite{Scope: draftScope(scope), SolutionId: id, CommandJson: string(body)}
	opts := []grpc.CallOption{grpc.MaxCallRecvMsgSize(1024*1024 + 1024)}
	var raw *pb.SolutionResponse
	var err error
	switch operation {
	case "create":
		raw, err = c.RPC.Create(ctx, command, opts...)
	case "save":
		raw, err = c.RPC.Save(ctx, command, opts...)
	case "prepare":
		raw, err = c.RPC.Prepare(ctx, command, opts...)
	default:
		return nil, app.ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	return solutionPayload(raw, "qs-ai-solution/v1", id)
}
