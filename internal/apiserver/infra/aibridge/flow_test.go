package aibridge

import (
	"context"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

type flowRPC struct {
	pb.FlowManagementClient
	reply    *pb.SolutionResponse
	err      error
	query    *pb.FlowQuery
	deadline time.Time
}

func (f *flowRPC) GetSolution(ctx context.Context, q *pb.FlowQuery, _ ...grpc.CallOption) (*pb.SolutionResponse, error) {
	f.query = q
	f.deadline, _ = ctx.Deadline()
	return f.reply, f.err
}
func TestFlowClientPreservesScopeDeadlineAndRejectsMismatchedEvidence(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000003"
	scope := app.DraftScope{OrganizationID: 12, OperatorUserID: 34}
	rpc := &flowRPC{reply: &pb.SolutionResponse{SchemaVersion: "qs-ai-flow/v1", DataJson: `{"schema_version":"qs-ai-flow/v1","source_kind":"solution","source_id":"` + id + `"}`}}
	client := FlowClient{RPC: rpc}
	if _, err := client.ReadFlow(context.Background(), scope, "solution", id); err != nil {
		t.Fatal(err)
	}
	if rpc.query.Scope.OrganizationId != 12 || rpc.query.Scope.OperatorUserId != 34 || rpc.query.SolutionId != id || rpc.deadline.IsZero() || time.Until(rpc.deadline) > 3*time.Second {
		t.Fatal("scope or deadline missing")
	}
	rpc.reply.DataJson = `{"schema_version":"qs-ai-flow/v1","source_kind":"publication","source_id":"` + id + `"}`
	if _, err := client.ReadFlow(context.Background(), scope, "solution", id); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	rpc.err = status.Error(codes.PermissionDenied, "scope denied")
	if _, err := client.ReadFlow(context.Background(), scope, "solution", id); status.Code(err) != codes.PermissionDenied {
		t.Fatal(err)
	}
}
