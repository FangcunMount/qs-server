package service

import (
	"context"
	"testing"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestAIExplanationPromptEvaluationAutomationRejectsLegacyRunExecution(t *testing.T) {
	runner := &promptEvaluationRunnerStub{}
	service := NewAIExplanationAutomationService(nil, runner)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-event-id", "event-1701"))
	_, err := service.ExecutePromptEvaluationStep(ctx, &interpretationpb.ExecutePromptEvaluationStepRequest{
		OrgId: 12, RunId: "1701", CaseId: "g1", Attempt: 1, RequestedBy: "user:42", EventId: "event-1701",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("legacy execution code = %s, err=%v", status.Code(err), err)
	}
}

func TestAIExplanationPromptEvaluationAutomationKeepsLegacyRecheckExecution(t *testing.T) {
	runner := &promptEvaluationRunnerStub{recheckErr: appevaluation.ErrAttemptExecutionBusy}
	service := NewAIExplanationAutomationService(nil, runner)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-event-id", "event-1701"))
	_, err := service.ExecutePromptEvaluationStep(ctx, &interpretationpb.ExecutePromptEvaluationStepRequest{
		OrgId: 12, RunId: "1701", RecheckId: "1801", CaseId: "g1", Attempt: 1, RequestedBy: "user:42", EventId: "event-1701",
	})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("busy lease code = %s, err=%v", status.Code(err), err)
	}
	if runner.recheckCommand.RecheckID != meta.ID(1801) || runner.recheckCommand.SourceRunID != meta.ID(1701) ||
		runner.recheckCommand.Owner != "event-1701" {
		t.Fatalf("recheck command = %#v", runner.recheckCommand)
	}

	badCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-event-id", "different-event"))
	_, err = service.ExecutePromptEvaluationStep(badCtx, &interpretationpb.ExecutePromptEvaluationStepRequest{
		OrgId: 12, RunId: "1701", CaseId: "g1", Attempt: 1, RequestedBy: "user:42", EventId: "event-1701",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("mismatched event metadata code = %s, err=%v", status.Code(err), err)
	}
}

func TestAIExplanationPromptEvaluationAutomationRoutesExactV2Address(t *testing.T) {
	runner := &promptEvaluationRunnerStub{v2Err: appevaluation.ErrAttemptExecutionBusy}
	service := NewAIExplanationAutomationService(nil, runner)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-event-id", "event-v2-1701"))
	_, err := service.ExecutePromptEvaluationStep(ctx, &interpretationpb.ExecutePromptEvaluationStepRequest{
		OrgId: 12, RunId: "1701", CaseId: "g1", RequestedBy: "user:42", EventId: "event-v2-1701",
		EvidenceVersion: "v2", ExecutionKind: "semantic", SlotOrdinal: 2,
		CandidateId: "candidate:g1:2", ExecutionOrdinal: 1,
	})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("v2 busy lease code = %s, err=%v", status.Code(err), err)
	}
	if runner.v2Command.RunID != meta.ID(1701) || runner.v2Command.ExecutionKind != domainevaluation.EvidenceExecutionSemantic ||
		runner.v2Command.CaseID != "g1" || runner.v2Command.SlotOrdinal != 2 || runner.v2Command.CandidateID != "candidate:g1:2" ||
		runner.v2Command.ExecutionOrdinal != 1 || runner.v2Command.Owner != "event-v2-1701" ||
		runner.v2Command.RequestedOrgID != 12 || runner.v2Command.RequestedBy != "user:42" {
		t.Fatalf("v2 command = %#v", runner.v2Command)
	}
}

type promptEvaluationRunnerStub struct {
	recheckCommand appevaluation.RunRecheckCommand
	recheckResult  *appevaluation.OnlineRecheckResult
	recheckErr     error
	v2Command      appevaluation.OnlineStepV2Command
	v2Result       *appevaluation.OnlineStepV2Result
	v2Err          error
}

func (s *promptEvaluationRunnerStub) RunRecheckV1(_ context.Context, command appevaluation.RunRecheckCommand) (*appevaluation.OnlineRecheckResult, error) {
	s.recheckCommand = command
	return s.recheckResult, s.recheckErr
}

func (s *promptEvaluationRunnerStub) RunStepV2(_ context.Context, command appevaluation.OnlineStepV2Command) (*appevaluation.OnlineStepV2Result, error) {
	s.v2Command = command
	return s.v2Result, s.v2Err
}
