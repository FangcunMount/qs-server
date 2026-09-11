package aiexplanation

import (
	"context"
	"strconv"

	aiport "github.com/FangcunMount/qs-server/internal/collection-server/port/aiexplanation"
	"github.com/google/uuid"
)

type WorkflowRequest struct {
	RequestID string `json:"request_id" binding:"required"`
	ReportID  string `json:"report_id" binding:"required"`
}
type WorkflowAccepted = aiport.WorkflowAccepted

func (s *Service) RequestWorkflow(ctx context.Context, testeeID, assessmentID uint64, request WorkflowRequest) (*WorkflowAccepted, error) {
	id, err := uuid.Parse(request.RequestID)
	if err != nil || id.String() != request.RequestID || testeeID == 0 || assessmentID == 0 {
		return nil, ErrInvalidRequest
	}
	reportID, err := strconv.ParseUint(request.ReportID, 10, 64)
	if err != nil || reportID == 0 || strconv.FormatUint(reportID, 10) != request.ReportID {
		return nil, ErrInvalidRequest
	}
	client, ok := s.client.(aiport.WorkflowClient)
	if !ok {
		return nil, ErrUnavailable
	}
	result, err := client.RequestWorkflow(ctx, testeeID, assessmentID, reportID, request.RequestID)
	if err != nil {
		return nil, err
	}
	if result == nil || result.RequestID != request.RequestID || result.Status != "accepted" {
		return nil, ErrUnavailable
	}
	return result, nil
}

type WorkflowResult = aiport.WorkflowResult

func (s *Service) GetWorkflow(ctx context.Context, testeeID, assessmentID uint64, requestID string) (*WorkflowResult, error) {
	id, err := uuid.Parse(requestID)
	if err != nil || id.String() != requestID || id == uuid.Nil || testeeID == 0 || assessmentID == 0 {
		return nil, ErrInvalidRequest
	}
	client, ok := s.client.(aiport.WorkflowReader)
	if !ok {
		return nil, ErrUnavailable
	}
	result, err := client.GetWorkflow(ctx, testeeID, assessmentID, requestID)
	if err != nil {
		return nil, err
	}
	if result == nil || result.RequestID != requestID {
		return nil, ErrUnavailable
	}
	return result, nil
}
