package grpcclient

import (
	"context"
	"testing"
	"time"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	"google.golang.org/grpc"
)

func TestAIExplanationAutomationClientUsesDedicatedTimeout(t *testing.T) {
	timeout := 2*time.Minute + 45*time.Second
	stub := &aiExplanationAutomationClientStub{}
	client := &AIExplanationAutomationClient{
		manager: &Manager{config: &ManagerConfig{Timeout: 30 * time.Second, AIExplanationTimeout: timeout}},
		client:  stub,
	}

	startedAt := time.Now()
	if _, err := client.ExecuteAIExplanation(context.Background(), &interpretationpb.ExecuteAIExplanationRequest{}); err != nil {
		t.Fatal(err)
	}
	assertDeadlineNear(t, stub.deadline, startedAt.Add(timeout))

	startedAt = time.Now()
	if _, err := client.ExecutePromptEvaluationStep(context.Background(), &interpretationpb.ExecutePromptEvaluationStepRequest{}); err != nil {
		t.Fatal(err)
	}
	assertDeadlineNear(t, stub.deadline, startedAt.Add(timeout))
}

func assertDeadlineNear(t *testing.T, got, want time.Time) {
	t.Helper()
	if got.IsZero() || got.Before(want.Add(-time.Second)) || got.After(want.Add(time.Second)) {
		t.Fatalf("deadline = %s, want near %s", got, want)
	}
}

type aiExplanationAutomationClientStub struct {
	deadline time.Time
}

func (s *aiExplanationAutomationClientStub) ExecuteAIExplanation(ctx context.Context, _ *interpretationpb.ExecuteAIExplanationRequest, _ ...grpc.CallOption) (*interpretationpb.ExecuteAIExplanationResponse, error) {
	s.deadline, _ = ctx.Deadline()
	return &interpretationpb.ExecuteAIExplanationResponse{}, nil
}

func (s *aiExplanationAutomationClientStub) ExecutePromptEvaluationStep(ctx context.Context, _ *interpretationpb.ExecutePromptEvaluationStepRequest, _ ...grpc.CallOption) (*interpretationpb.ExecutePromptEvaluationStepResponse, error) {
	s.deadline, _ = ctx.Deadline()
	return &interpretationpb.ExecutePromptEvaluationStepResponse{}, nil
}

var _ interpretationpb.AIExplanationAutomationServiceClient = (*aiExplanationAutomationClientStub)(nil)
