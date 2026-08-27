package service

import (
	"context"
	"testing"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	aiexecution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/execution"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestAIExplanationAutomationMapsActiveCapacityToResourceExhausted(t *testing.T) {
	service := NewAIExplanationAutomationService(aiExplanationExecutorErrorStub{err: domaingeneration.ErrAssessmentActiveCapacityExceeded}, nil)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-event-id", "event-1701"))
	_, err := service.ExecuteAIExplanation(ctx, &interpretationpb.ExecuteAIExplanationRequest{
		GenerationId: "1701", TraceId: "event-1701",
	})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("active capacity code = %s, err=%v", status.Code(err), err)
	}
}

func TestAIExplanationAutomationRejectsPartialOrStaleRetryProof(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-event-id", "event-1701"))
	service := NewAIExplanationAutomationService(aiExplanationExecutorErrorStub{}, nil)
	_, err := service.ExecuteAIExplanation(ctx, &interpretationpb.ExecuteAIExplanationRequest{
		GenerationId: "1701", TraceId: "event-1701", EventId: "event-1701", ExpectedAttempt: 1,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("partial retry proof code = %s, err=%v", status.Code(err), err)
	}

	service = NewAIExplanationAutomationService(aiExplanationExecutorErrorStub{err: domainrun.ErrRetryNotAllowed}, nil)
	_, err = service.ExecuteAIExplanation(ctx, &interpretationpb.ExecuteAIExplanationRequest{
		GenerationId: "1701", TraceId: "event-1701", EventId: "event-1701", ExpectedAttempt: 1,
		AttemptOrigin: "manual", ActionRequestId: "retry-request-1",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("stale retry proof code = %s, err=%v", status.Code(err), err)
	}
}

func TestAIExplanationAutomationRejectsPartialOrStaleLeaseRecoveryProof(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-event-id", "event-1701"))
	service := NewAIExplanationAutomationService(aiExplanationExecutorErrorStub{}, nil)
	_, err := service.ExecuteAIExplanation(ctx, &interpretationpb.ExecuteAIExplanationRequest{
		GenerationId: "1701", TraceId: "event-1701", EventId: "event-1701", ExpectedRunId: "1801",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("partial lease recovery proof code = %s, err=%v", status.Code(err), err)
	}

	service = NewAIExplanationAutomationService(aiExplanationExecutorErrorStub{err: domainrun.ErrRecoveryNotAllowed}, nil)
	_, err = service.ExecuteAIExplanation(ctx, &interpretationpb.ExecuteAIExplanationRequest{
		GenerationId: "1701", TraceId: "event-1701", EventId: "event-1701", ExpectedRunId: "1801",
		ExpectedLeaseExpiresAt: "2026-08-27T01:04:00Z", ExpectedInvocationPhase: "prepared",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("stale lease recovery proof code = %s, err=%v", status.Code(err), err)
	}
}

type aiExplanationExecutorErrorStub struct {
	err error
}

func (s aiExplanationExecutorErrorStub) Execute(context.Context, aiexecution.Command) (*aiexecution.Result, error) {
	return nil, s.err
}
