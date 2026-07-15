package behaviorassessment

import (
	"context"
	"errors"
	"fmt"
	"time"

	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
)

var ErrNotBehaviorAssessment = errors.New("assessment is not a behavior ability evaluation")

var modelKinds = []string{"behavioral_rating", "cognitive"}

type queryReader interface {
	ListMyAssessmentsByModelKinds(ctx context.Context, testeeID uint64, status string, modelKinds []string, page, pageSize int32) (*evaluationapp.ListAssessmentsResponse, error)
	GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*evaluationapp.AssessmentDetailResponse, error)
	GetAssessmentReport(ctx context.Context, testeeID, assessmentID uint64) (*evaluationapp.AssessmentReportResponse, error)
}

type QueryService struct {
	reader     queryReader
	waitReport *reportwait.Service
}

func NewQueryService(reader queryReader, waitReport *reportwait.Service) *QueryService {
	return &QueryService{reader: reader, waitReport: waitReport}
}

func (s *QueryService) List(ctx context.Context, testeeID uint64, req *ListAssessmentsRequest) (*ListAssessmentsResponse, error) {
	page, pageSize := evaluationapp.NormalizeListPage(req.Page, req.PageSize, evaluationapp.AssessmentListPageDefault)
	return s.reader.ListMyAssessmentsByModelKinds(ctx, testeeID, req.Status, modelKinds, page, pageSize)
}

func (s *QueryService) Get(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailResponse, error) {
	result, err := s.reader.GetMyAssessment(ctx, testeeID, assessmentID)
	if err != nil || result == nil {
		return result, err
	}
	if !isBehaviorAbilityModel(result.Model) {
		return nil, ErrNotBehaviorAssessment
	}
	return result, nil
}

func (s *QueryService) GetReport(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentReportResponse, error) {
	result, err := s.reader.GetAssessmentReport(ctx, testeeID, assessmentID)
	if err != nil || result == nil {
		return result, err
	}
	if !isBehaviorAbilityModel(result.Model) {
		return nil, ErrNotBehaviorAssessment
	}
	return result, nil
}

func (s *QueryService) GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentStatusResponse, error) {
	if _, err := s.Get(ctx, testeeID, assessmentID); err != nil {
		return nil, err
	}
	return s.currentStatus(ctx, testeeID, assessmentID, false, 0)
}

func (s *QueryService) WaitReport(ctx context.Context, testeeID, assessmentID uint64, timeout time.Duration) (*AssessmentStatusResponse, error) {
	if _, err := s.Get(ctx, testeeID, assessmentID); err != nil {
		return nil, err
	}
	return s.currentStatus(ctx, testeeID, assessmentID, true, timeout)
}

func (s *QueryService) currentStatus(ctx context.Context, testeeID, assessmentID uint64, wait bool, timeout time.Duration) (*AssessmentStatusResponse, error) {
	if s.waitReport == nil {
		return nil, fmt.Errorf("wait report service is not configured")
	}
	var (
		status *evaluationapp.AssessmentStatusResponse
		err    error
	)
	if wait {
		status, err = s.waitReport.Wait(ctx, testeeID, assessmentID, timeout)
	} else {
		status, err = s.waitReport.GetStatus(ctx, testeeID, assessmentID)
	}
	if err != nil {
		return nil, err
	}
	public := reportstatus.ToPublicAssessmentStatus(status)
	if public == nil {
		return nil, nil
	}
	response := &AssessmentStatusResponse{Status: public.Status, Stage: public.Stage, Message: public.Message, Reason: public.Reason, NextPollAfterMs: public.NextPollAfterMs, UpdatedAt: public.UpdatedAt}
	if public.Status == "interpreted" {
		if detail, err := s.Get(ctx, testeeID, assessmentID); err == nil && detail != nil {
			model := detail.Model
			response.Model = &model
			response.Level = detail.Level
		}
	}
	return response, nil
}

func isBehaviorAbilityModel(model evaluationapp.ModelIdentityResponse) bool {
	return model.Kind == "behavioral_rating" || model.Kind == "cognitive"
}
