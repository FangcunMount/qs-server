package aibridge

import (
	"context"
	"errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"testing"
	"time"
)

type evaluationRPCStub struct {
	finalCommand      *pb.EvaluationFinalizeCommand
	finalState        *pb.EvaluationState
	gateQuery         *pb.EvaluationGateQuery
	gatePreview       *pb.EvaluationGatePreview
	candidateIndex    *pb.EvaluationCandidateIndex
	candidateEvidence *pb.EvaluationCandidateEvidence
	candidateQuery    *pb.EvaluationCandidateQuery
	review            *pb.EvaluationReviewCommand
	create            *pb.EvaluationCreateCommand
	calls             int
	command           *pb.UnknownResolutionCommand
	fail              error
	t                 *testing.T
}

func (s *evaluationRPCStub) Get(ctx context.Context, r *pb.EvaluationQuery, _ ...grpc.CallOption) (*pb.EvaluationState, error) {
	return &pb.EvaluationState{RunId: r.RunId, Version: 7, Status: "collecting", ResolutionsJson: "[]"}, nil
}
func (s *evaluationRPCStub) ResolveUnknown(ctx context.Context, r *pb.UnknownResolutionCommand, _ ...grpc.CallOption) (*pb.EvaluationState, error) {
	s.calls++
	s.command = r
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("bounded timeout required")
	}
	if s.fail != nil {
		return nil, s.fail
	}
	return s.Get(ctx, r.Scope)
}
func TestEvaluationClientForwardsConfirmedCommandOnce(t *testing.T) {
	rpc := &evaluationRPCStub{t: t}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "run:1", OrganizationID: 7, OperatorUserID: 42}
	command := app.UnknownResolution{ExpectedVersion: 6, ExecutionID: "execution:1", Decision: "cancel_run", Reason: "人工核对", Confirm: true, AcknowledgedDuplicateCallAndCostRisk: true}
	result, err := client.ResolveUnknown(context.Background(), scope, command)
	if err != nil || result.Version != 7 || rpc.calls != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	if rpc.command.Scope.OrganizationId != 7 || rpc.command.Scope.OperatorUserId != 42 || rpc.command.ExecutionId != command.ExecutionID || !rpc.command.Confirm || !rpc.command.AcknowledgedDuplicateCallAndCostRisk {
		t.Fatal("command drift")
	}
	rpc.fail = context.DeadlineExceeded
	if _, err = client.ResolveUnknown(context.Background(), scope, command); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if rpc.calls != 2 {
		t.Fatal("transport retried uncertain mutation")
	}
}

func (s *evaluationRPCStub) Start(ctx context.Context, r *pb.EvaluationStartCommand, _ ...grpc.CallOption) (*pb.EvaluationState, error) {
	return s.ResolveUnknown(ctx, &pb.UnknownResolutionCommand{Scope: r.Scope, ExpectedVersion: r.ExpectedVersion, Reason: r.Reason, Confirm: r.Confirm})
}

func TestStartForwardsOnceAndDoesNotRetryUnknownOutcome(t *testing.T) {
	rpc := &evaluationRPCStub{t: t}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "run:1", OrganizationID: 7, OperatorUserID: 42}
	command := app.EvaluationStart{ExpectedVersion: 1, Reason: "启动评测", Confirm: true}
	if _, err := client.StartEvaluation(context.Background(), scope, command); err != nil {
		t.Fatal(err)
	}
	if rpc.calls != 1 || rpc.command.Scope.OperatorUserId != 42 || rpc.command.ExpectedVersion != 1 || rpc.command.Reason != command.Reason || !rpc.command.Confirm {
		t.Fatal("start command drift")
	}
	rpc.fail = context.DeadlineExceeded
	if _, err := client.StartEvaluation(context.Background(), scope, command); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if rpc.calls != 2 {
		t.Fatal("uncertain start retried")
	}
}

func (s *evaluationRPCStub) Create(ctx context.Context, r *pb.EvaluationCreateCommand, _ ...grpc.CallOption) (*pb.EvaluationState, error) {
	s.create = r
	return s.ResolveUnknown(ctx, &pb.UnknownResolutionCommand{Scope: r.Scope, Reason: r.Reason, Confirm: r.Confirm})
}
