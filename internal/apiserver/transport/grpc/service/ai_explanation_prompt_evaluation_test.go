package service

import (
	"context"
	"testing"
	"time"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestAIExplanationPromptEvaluationAutomationUsesEventAddressAndAcksCancellation(t *testing.T) {
	now := time.Date(2026, 8, 27, 16, 0, 0, 0, time.UTC)
	runRecord, err := domainevaluation.NewRequested(meta.ID(1701), automationEvaluationRelease(), 12, "user:42", "evaluate release", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.Cancel("user:42", "stop before dispatch", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	runner := &promptEvaluationRunnerStub{result: &appevaluation.OnlineStepResult{Status: appevaluation.OnlineStepCanceled, Run: runRecord}}
	service := NewAIExplanationAutomationService(nil, runner)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-event-id", "event-1701"))
	response, err := service.ExecutePromptEvaluationStep(ctx, &interpretationpb.ExecutePromptEvaluationStepRequest{
		OrgId: 12, RunId: "1701", CaseId: "g1", Attempt: 1, RequestedBy: "user:42", EventId: "event-1701",
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.GetStatus() != "canceled" || response.GetRunStatus() != "canceled" || !response.GetSuccess() ||
		runner.command.Owner != "event-1701" || runner.command.RequestedOrgID != 12 || runner.command.RequestedBy != "user:42" {
		t.Fatalf("response/command = %#v / %#v", response, runner.command)
	}
}

func TestAIExplanationPromptEvaluationAutomationMapsBusyLeaseToAborted(t *testing.T) {
	runner := &promptEvaluationRunnerStub{err: appevaluation.ErrAttemptExecutionBusy}
	service := NewAIExplanationAutomationService(nil, runner)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-event-id", "event-1701"))
	_, err := service.ExecutePromptEvaluationStep(ctx, &interpretationpb.ExecutePromptEvaluationStepRequest{
		OrgId: 12, RunId: "1701", CaseId: "g1", Attempt: 1, RequestedBy: "user:42", EventId: "event-1701",
	})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("busy lease code = %s, err=%v", status.Code(err), err)
	}

	badCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-event-id", "different-event"))
	_, err = service.ExecutePromptEvaluationStep(badCtx, &interpretationpb.ExecutePromptEvaluationStepRequest{
		OrgId: 12, RunId: "1701", CaseId: "g1", Attempt: 1, RequestedBy: "user:42", EventId: "event-1701",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("mismatched event metadata code = %s, err=%v", status.Code(err), err)
	}
}

type promptEvaluationRunnerStub struct {
	command appevaluation.OnlineStepCommand
	result  *appevaluation.OnlineStepResult
	err     error
}

func (s *promptEvaluationRunnerStub) RunStepV1(_ context.Context, command appevaluation.OnlineStepCommand) (*appevaluation.OnlineStepResult, error) {
	s.command = command
	return s.result, s.err
}

func automationEvaluationRelease() domainevaluation.ReleaseIdentity {
	return domainevaluation.ReleaseIdentity{
		Suite:        domainevaluation.SuiteRef{ID: "suite-v1", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("suite")), GitBlobSHA: "suite-blob"},
		Prompt:       aiexplanation.PromptRef{TemplateID: "prompt", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "prompt-blob"},
		Profile:      aiexplanation.ProfileRef{ID: "profile", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("profile"))},
		InputSchema:  domainevaluation.SchemaRef{Version: aiexplanation.InputSchemaVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("input"))},
		OutputSchema: domainevaluation.SchemaRef{Version: aiexplanation.OutputSchemaVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("output"))},
		Provider:     aiexplanation.ProviderExecutionSpec{Route: "route", RouteRevision: "v1", ResolvedProvider: "provider", ResolvedModel: "model", Fingerprint: aiexplanation.NewFingerprint([]byte("route"))},
		Decoding:     domainevaluation.DecodingParameters{MaxOutputTokens: 1000},
		SemanticEvaluator: domainevaluation.SemanticEvaluatorSpec{
			Version: "judge-v1", Prompt: aiexplanation.PromptRef{TemplateID: "judge", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("judge")), GitBlobSHA: "judge-blob"},
			OutputSchema: domainevaluation.SchemaRef{Version: "judge-output/v1", Fingerprint: aiexplanation.NewFingerprint([]byte("judge-output"))},
			Provider:     aiexplanation.ProviderExecutionSpec{Route: "judge", RouteRevision: "v1", ResolvedProvider: "provider", ResolvedModel: "judge-model", Fingerprint: aiexplanation.NewFingerprint([]byte("judge-route"))},
			Decoding:     domainevaluation.DecodingParameters{MaxOutputTokens: 1000},
		},
		GenerationCaseIDs: []string{"g1", "g2", "g3", "g4", "g5", "g6", "g7"},
		PreflightCaseID:   "p1", PreflightRejectionReason: "ineligible", RepetitionsPerCase: 5,
	}
}
