package typologyassessment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	"github.com/FangcunMount/qs-server/internal/pkg/cancelerr"
)

const personalityModelKind = "personality"

var errNotTypologyAssessment = errors.New("assessment is not a personality evaluation")

type QueryService struct {
	evaluationClient evaluationapp.BFFReader
	waitReport       *reportwait.Service
}

func NewQueryService(evaluationClient evaluationapp.BFFReader, waitReport *reportwait.Service) *QueryService {
	return &QueryService{
		evaluationClient: evaluationClient,
		waitReport:       waitReport,
	}
}

func (s *QueryService) List(ctx context.Context, testeeID uint64, req *ListAssessmentsRequest) (*ListAssessmentsResponse, error) {
	page, pageSize := evaluationapp.NormalizeListPage(req.Page, req.PageSize, evaluationapp.AssessmentListPageDefault)
	result, err := s.evaluationClient.ListMyAssessments(ctx, testeeID, req.Status, "", "", personalityModelKind, req.Algorithm, "", "", page, pageSize)
	if err != nil {
		logTypologyAssessmentError("list typology assessments failed", err)
		return nil, err
	}
	return result, nil
}

func (s *QueryService) Get(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailResponse, error) {
	result, err := s.evaluationClient.GetMyAssessment(ctx, testeeID, assessmentID)
	if err != nil {
		logTypologyAssessmentError("get typology assessment failed", err)
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	if err := ensureTypologyModel(result.Model); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *QueryService) GetReport(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentReportResponse, error) {
	result, err := s.evaluationClient.GetAssessmentReport(ctx, testeeID, assessmentID)
	if err != nil {
		logTypologyAssessmentError("get typology assessment report failed", err)
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	if err := ensureTypologyModel(result.Model); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *QueryService) GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentStatusResponse, error) {
	if s.waitReport == nil {
		return nil, fmt.Errorf("wait report service is not configured")
	}
	status, err := s.waitReport.GetStatus(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return s.toPublicStatusResponse(ctx, testeeID, assessmentID, status), nil
}

func (s *QueryService) WaitReport(ctx context.Context, testeeID, assessmentID uint64, timeout time.Duration) (*AssessmentStatusResponse, error) {
	if s.waitReport == nil {
		return nil, fmt.Errorf("wait report service is not configured")
	}
	status, err := s.waitReport.Wait(ctx, testeeID, assessmentID, timeout)
	if err != nil {
		return nil, err
	}
	return s.toPublicStatusResponse(ctx, testeeID, assessmentID, status), nil
}

func (s *QueryService) toPublicStatusResponse(
	ctx context.Context,
	testeeID, assessmentID uint64,
	status *evaluationapp.AssessmentStatusResponse,
) *AssessmentStatusResponse {
	pub := reportstatus.ToPublicAssessmentStatus(status)
	resp := &AssessmentStatusResponse{
		Status:          pub.Status,
		Stage:           pub.Stage,
		Message:         pub.Message,
		Reason:          pub.Reason,
		NextPollAfterMs: pub.NextPollAfterMs,
		UpdatedAt:       pub.UpdatedAt,
	}
	if pub.Status == "interpreted" {
		if detail, err := s.Get(ctx, testeeID, assessmentID); err == nil && detail != nil {
			model := detail.Model
			resp.Model = &model
			resp.Level = detail.Level
		}
	}
	return resp
}

func ensureTypologyModel(model evaluationapp.ModelIdentityResponse) error {
	if model.Kind != personalityModelKind {
		return errNotTypologyAssessment
	}
	return nil
}

func logTypologyAssessmentError(message string, err error) {
	if cancelerr.Is(err) {
		log.Debugf("%s: %v", message, err)
		return
	}
	log.Errorf("%s: %v", message, err)
}

func IsNotTypologyAssessment(err error) bool {
	return errors.Is(err, errNotTypologyAssessment)
}
