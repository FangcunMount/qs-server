package grpcclient

import (
	"context"
	"encoding/json"
	"fmt"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	aiport "github.com/FangcunMount/qs-server/internal/collection-server/port/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
)

type ParticipantAIExplanationClient struct {
	client  *Client
	service interpretationpb.ParticipantAIExplanationServiceClient
	signer  *delegatedsubject.Signer
}

func NewParticipantAIExplanationClient(client *Client, signer *delegatedsubject.Signer) *ParticipantAIExplanationClient {
	return &ParticipantAIExplanationClient{
		client: client, service: interpretationpb.NewParticipantAIExplanationServiceClient(client.Conn()), signer: signer,
	}
}

func (c *ParticipantAIExplanationClient) attachDelegatedSubject(ctx context.Context, testeeID uint64, purpose string) (context.Context, error) {
	if c == nil || c.signer == nil || !c.signer.Enabled() {
		return ctx, nil
	}
	input, err := delegatedsubject.SignInputFromContext(ctx, testeeID, purpose, 0)
	if err != nil {
		return ctx, err
	}
	return delegatedsubject.AppendToOutgoingContext(ctx, c.signer, input)
}

func (c *ParticipantAIExplanationClient) RequestWorkflow(ctx context.Context, testeeID, assessmentID, reportID uint64, requestID string) (*aiport.WorkflowAccepted, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	ctx, err := c.attachDelegatedSubject(ctx, testeeID, delegatedsubject.PurposeAIExplanationRequest)
	if err != nil {
		return nil, err
	}
	result, err := c.service.RequestAIWorkflow(ctx, &interpretationpb.RequestAIWorkflowRequest{TesteeId: testeeID, AssessmentId: assessmentID, ReportId: reportID, RequestId: requestID})
	if err != nil {
		return nil, err
	}
	return &aiport.WorkflowAccepted{RequestID: result.RequestId, Status: result.Status}, nil
}

func (c *ParticipantAIExplanationClient) GetWorkflow(ctx context.Context, testeeID, assessmentID uint64, requestID string) (*aiport.WorkflowResult, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	ctx, err := c.attachDelegatedSubject(ctx, testeeID, delegatedsubject.PurposeAIExplanationGet)
	if err != nil {
		return nil, err
	}
	result, err := c.service.GetAIWorkflow(ctx, &interpretationpb.GetAIWorkflowRequest{TesteeId: testeeID, AssessmentId: assessmentID, RequestId: requestID})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("missing AI workflow result")
	}
	if result.ContentJson != "" && !json.Valid([]byte(result.ContentJson)) {
		return nil, fmt.Errorf("invalid AI workflow content")
	}
	if result.ReferenceMaterialJson != "" && !json.Valid([]byte(result.ReferenceMaterialJson)) {
		return nil, fmt.Errorf("invalid AI workflow references")
	}
	return &aiport.WorkflowResult{RequestID: result.RequestId, Status: result.Status, Version: result.Version, Content: json.RawMessage(result.ContentJson), ArtifactID: result.ArtifactId, ReportID: result.ReportId, SourceVersion: result.SourceVersion, ReferenceMaterial: json.RawMessage(result.ReferenceMaterialJson), ReferenceMaterialFingerprint: result.ReferenceMaterialFingerprint}, nil
}

func (c *ParticipantAIExplanationClient) GetWorkflowSource(ctx context.Context, testeeID, assessmentID uint64) (*aiport.WorkflowSource, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	ctx, err := c.attachDelegatedSubject(ctx, testeeID, delegatedsubject.PurposeAIExplanationCapability)
	if err != nil {
		return nil, err
	}
	result, err := c.service.GetAIWorkflowSource(ctx, &interpretationpb.GetAIWorkflowSourceRequest{TesteeId: testeeID, AssessmentId: assessmentID})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("missing AI workflow source")
	}
	response := &aiport.WorkflowSource{Status: result.Status, ReportID: result.ReportId, SourceVersion: result.SourceVersion}
	if result.AiEligibility != nil {
		response.AIEligibility = &aiport.WorkflowEligibility{Status: result.AiEligibility.Status, ReasonCode: result.AiEligibility.ReasonCode}
	}
	return response, nil
}

func (c *ParticipantAIExplanationClient) GetWorkflowOperation(ctx context.Context, testeeID, assessmentID uint64, requestID, commandID string) (*aiport.WorkflowOperation, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	ctx, err := c.attachDelegatedSubject(ctx, testeeID, delegatedsubject.PurposeAIExplanationGet)
	if err != nil {
		return nil, err
	}
	result, err := c.service.GetAIWorkflow(ctx, &interpretationpb.GetAIWorkflowRequest{TesteeId: testeeID, AssessmentId: assessmentID, RequestId: requestID, CommandId: commandID})
	if err != nil {
		return nil, err
	}
	if result == nil || result.RequestId != requestID || result.Operation == nil {
		return nil, fmt.Errorf("invalid operation response")
	}
	o := result.Operation
	if len(o.ReceiptJson) > 0 && !json.Valid(o.ReceiptJson) {
		return nil, fmt.Errorf("invalid operation response")
	}
	return &aiport.WorkflowOperation{OperationID: o.OperationId, CommandID: o.CommandId, Status: o.Status, TransportStatus: o.TransportStatus, Decision: o.Decision, Code: o.Code, ResourceID: o.ResourceId, Receipt: append([]byte(nil), o.ReceiptJson...)}, nil
}
