package grpcclient

import (
	"context"
	"fmt"

	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
)

type AssessmentIntakeClient struct {
	manager *Manager
	client  evalpb.AssessmentIntakeServiceClient
}

func NewAssessmentIntakeClient(manager *Manager) *AssessmentIntakeClient {
	return &AssessmentIntakeClient{manager: manager, client: evalpb.NewAssessmentIntakeServiceClient(manager.Conn())}
}
func (c *AssessmentIntakeClient) EnsureAssessment(ctx context.Context, req *evalpb.EnsureAssessmentRequest) (*evalpb.EnsureAssessmentResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()
	resp, err := c.client.EnsureAssessment(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure assessment: %w", err)
	}
	return resp, nil
}

type EvaluationWorkerClient struct {
	manager *Manager
	client  evalpb.EvaluationWorkerServiceClient
}

func NewEvaluationWorkerClient(manager *Manager) *EvaluationWorkerClient {
	return &EvaluationWorkerClient{manager: manager, client: evalpb.NewEvaluationWorkerServiceClient(manager.Conn())}
}
func (c *EvaluationWorkerClient) ExecuteEvaluation(ctx context.Context, assessmentID uint64) (*evalpb.ExecuteEvaluationResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()
	req := &evalpb.ExecuteEvaluationRequest{AssessmentId: assessmentID}
	resp, err := c.client.ExecuteEvaluation(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute evaluation: %w", err)
	}
	return resp, nil
}

type InterpretationAutomationClient struct {
	manager *Manager
	client  interpretationpb.InterpretationAutomationServiceClient
}

type AIExplanationAutomationClient struct {
	manager *Manager
	client  interpretationpb.AIExplanationAutomationServiceClient
}

func NewAIExplanationAutomationClient(manager *Manager) *AIExplanationAutomationClient {
	return &AIExplanationAutomationClient{manager: manager, client: interpretationpb.NewAIExplanationAutomationServiceClient(manager.Conn())}
}

func (c *AIExplanationAutomationClient) ExecuteAIExplanation(ctx context.Context, req *interpretationpb.ExecuteAIExplanationRequest) (*interpretationpb.ExecuteAIExplanationResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()
	resp, err := c.client.ExecuteAIExplanation(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute AI explanation: %w", err)
	}
	return resp, nil
}

func (c *AIExplanationAutomationClient) ExecutePromptEvaluationStep(ctx context.Context, request *interpretationpb.ExecutePromptEvaluationStepRequest) (*interpretationpb.ExecutePromptEvaluationStepResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()
	resp, err := c.client.ExecutePromptEvaluationStep(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to execute AI explanation prompt evaluation step: %w", err)
	}
	return resp, nil
}

func NewInterpretationAutomationClient(manager *Manager) *InterpretationAutomationClient {
	return &InterpretationAutomationClient{manager: manager, client: interpretationpb.NewInterpretationAutomationServiceClient(manager.Conn())}
}
func (c *InterpretationAutomationClient) GenerateReportFromOutcome(ctx context.Context, outcomeID string) (*interpretationpb.GenerateReportFromAssessmentResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()
	req := &interpretationpb.GenerateReportFromOutcomeRequest{OutcomeId: outcomeID}
	resp, err := c.client.GenerateReportFromOutcome(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate report from outcome: %w", err)
	}
	return resp, nil
}
