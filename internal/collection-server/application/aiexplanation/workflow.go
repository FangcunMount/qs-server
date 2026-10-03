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
	if err != nil || id.String() != request.RequestID || id == uuid.Nil || testeeID == 0 || assessmentID == 0 {
		return nil, ErrInvalidRequest
	}
	reportID, err := strconv.ParseUint(request.ReportID, 10, 64)
	if err != nil || reportID == 0 || strconv.FormatUint(reportID, 10) != request.ReportID {
		return nil, ErrInvalidRequest
	}
	client := s.client
	if client == nil {
		return nil, ErrUnavailable
	}
	result, err := client.RequestWorkflow(ctx, testeeID, assessmentID, reportID, request.RequestID)
	if err != nil {
		return nil, err
	}
	if result == nil || result.RequestID != request.RequestID || (result.Status != "accepted" && result.Status != "submitted") {
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
	client := s.client
	if client == nil {
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

type WorkflowSource = aiport.WorkflowSource

func (s *Service) GetWorkflowSource(ctx context.Context, testeeID, assessmentID uint64) (*WorkflowSource, error) {
	if testeeID == 0 || assessmentID == 0 {
		return nil, ErrInvalidRequest
	}
	client := s.client
	if client == nil {
		return nil, ErrUnavailable
	}
	result, err := client.GetWorkflowSource(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, ErrUnavailable
	}
	switch result.Status {
	case "ready":
		id, err := strconv.ParseUint(result.ReportID, 10, 64)
		if err != nil || id == 0 || strconv.FormatUint(id, 10) != result.ReportID || result.SourceVersion == "" {
			return nil, ErrUnavailable
		}
	case "not_ready", "not_applicable":
		if result.ReportID != "" || result.SourceVersion != "" {
			return nil, ErrUnavailable
		}
	default:
		return nil, ErrUnavailable
	}
	return result, nil
}

type WorkflowOperation = aiport.WorkflowOperation

func (s *Service) GetWorkflowOperation(ctx context.Context, testeeID, assessmentID uint64, requestID, commandID string) (*WorkflowOperation, error) {
	for _, raw := range []string{requestID, commandID} {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil || id.String() != raw {
			return nil, ErrInvalidRequest
		}
	}
	if testeeID == 0 || assessmentID == 0 {
		return nil, ErrInvalidRequest
	}
	reader, ok := s.client.(aiport.WorkflowOperationReader)
	if !ok {
		return nil, ErrUnavailable
	}
	o, err := reader.GetWorkflowOperation(ctx, testeeID, assessmentID, requestID, commandID)
	if err != nil {
		return nil, err
	}
	if o == nil || o.CommandID != commandID || o.OperationID != commandID {
		return nil, ErrUnavailable
	}
	return o, nil
}
