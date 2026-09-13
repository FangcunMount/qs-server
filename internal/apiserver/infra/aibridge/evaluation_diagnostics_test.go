package aibridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (s *evaluationRPCStub) ListExecutions(context.Context, *pb.EvaluationExecutionQuery, ...grpc.CallOption) (*pb.EvaluationExecutionPage, error) {
	s.t.Fatal("unexpected diagnostic list in another operation")
	return nil, app.ErrConflict
}
func (s *evaluationRPCStub) GetExecutionOutput(context.Context, *pb.EvaluationExecutionQuery, ...grpc.CallOption) (*pb.EvaluationExecutionOutput, error) {
	s.t.Fatal("unexpected diagnostic output in another operation")
	return nil, app.ErrConflict
}

type diagnosticRPC struct {
	pb.EvaluationManagementClient
	t       *testing.T
	request *pb.EvaluationExecutionQuery
	page    *pb.EvaluationExecutionPage
	output  *pb.EvaluationExecutionOutput
	calls   int
	err     error
}

func (s *diagnosticRPC) record(ctx context.Context, q *pb.EvaluationExecutionQuery) {
	s.calls++
	s.request = q
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("missing deadline")
	}
}
func (s *diagnosticRPC) ListExecutions(ctx context.Context, q *pb.EvaluationExecutionQuery, _ ...grpc.CallOption) (*pb.EvaluationExecutionPage, error) {
	s.record(ctx, q)
	return s.page, s.err
}
func (s *diagnosticRPC) GetExecutionOutput(ctx context.Context, q *pb.EvaluationExecutionQuery, _ ...grpc.CallOption) (*pb.EvaluationExecutionOutput, error) {
	s.record(ctx, q)
	return s.output, s.err
}
func diagnosticOutput() *pb.EvaluationExecutionOutput {
	raw := []byte{0xff, 0x00, 0x41}
	sum := sha256.Sum256(raw)
	empty := sha256.Sum256(nil)
	return &pb.EvaluationExecutionOutput{RunId: "run:1", Version: 7,
		Execution: &pb.EvaluationExecutionSummary{ExecutionId: "execution:1", InvocationId: "invocation:1", Kind: "generation", CaseId: "case:1", SlotOrdinal: 1, ExecutionOrdinal: 1, Status: "failed", RawOutputBytes: 3, EvidenceJson: `{"status":"failed","failure":{"code":"bad_output"}}`},
		RawOutput: raw, RawSha256: hex.EncodeToString(sum[:]), NormalizedSha256: hex.EncodeToString(empty[:])}
}
func TestDiagnosticClientPreservesBinaryOutputAndDoesNotRetry(t *testing.T) {
	rpc := &diagnosticRPC{t: t, output: diagnosticOutput()}
	rpc.page = &pb.EvaluationExecutionPage{RunId: "run:1", Version: 7, Executions: []*pb.EvaluationExecutionSummary{rpc.output.Execution}}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "run:1", OrganizationID: 7, OperatorUserID: 42}
	page, err := client.ListEvaluationExecutions(context.Background(), scope, app.ExecutionQuery{ExpectedVersion: 7, Limit: 20})
	if err != nil || len(page.Executions) != 1 || rpc.request.Scope.OrganizationId != 7 || rpc.request.Scope.OperatorUserId != 42 {
		t.Fatal(page, err, rpc.request)
	}
	output, err := client.GetEvaluationExecutionOutput(context.Background(), scope, 7, "execution:1")
	if err != nil || string(output.RawOutput) != string(rpc.output.RawOutput) || output.NormalizedOutput == nil {
		t.Fatal(output, err)
	}
	rpc.err = context.DeadlineExceeded
	_, err = client.GetEvaluationExecutionOutput(context.Background(), scope, 7, "execution:1")
	if !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 3 {
		t.Fatal("unexpected recovery/retry", rpc.calls, err)
	}
}
func TestDiagnosticClientRejectsWrongBindingAndContent(t *testing.T) {
	for _, name := range []string{"run", "version", "execution", "hash", "length", "status", "metadata", "nil"} {
		t.Run(name, func(t *testing.T) {
			raw := proto.Clone(diagnosticOutput()).(*pb.EvaluationExecutionOutput)
			switch name {
			case "run":
				raw.RunId = "other"
			case "version":
				raw.Version = 8
			case "execution":
				raw.Execution.ExecutionId = "other"
			case "hash":
				raw.RawOutput[0] = 0
			case "length":
				raw.Execution.RawOutputBytes = 0
			case "status":
				raw.Execution.Status = "approved"
			case "metadata":
				raw.Execution.EvidenceJson = `{"status":"succeeded"}`
			case "nil":
				raw.Execution = nil
			}
			client := &EvaluationClient{RPC: &diagnosticRPC{t: t, output: raw}}
			if _, err := client.GetEvaluationExecutionOutput(context.Background(), app.EvaluationScope{RunID: "run:1"}, 7, "execution:1"); !errors.Is(err, app.ErrConflict) {
				t.Fatal(err)
			}
		})
	}
}

func (s *evaluationRPCStub) GetCapacity(context.Context, *pb.PublicationScope, ...grpc.CallOption) (*pb.EvaluationCapacitySnapshot, error) {
	return nil, errors.New("unexpected capacity query")
}
