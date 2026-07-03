package personalityassessment

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

var errNotPersonalityAssessment = errors.New("assessment is not a personality evaluation")

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
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	result, err := s.evaluationClient.ListMyAssessmentsV2(ctx, testeeID, req.Status, "", "", personalityModelKind, req.Algorithm, page, pageSize)
	if err != nil {
		logPersonalityAssessmentError("list personality assessments failed", err)
		return nil, err
	}
	items := make([]AssessmentSummaryResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toAssessmentSummary(item))
	}
	return &ListAssessmentsResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}, nil
}

func (s *QueryService) Get(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailResponse, error) {
	result, err := s.evaluationClient.GetMyAssessmentV2(ctx, testeeID, assessmentID)
	if err != nil {
		logPersonalityAssessmentError("get personality assessment failed", err)
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	if err := ensurePersonalityModel(result.Model); err != nil {
		return nil, err
	}
	return toAssessmentDetail(result), nil
}

func (s *QueryService) GetReport(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentReportResponse, error) {
	result, err := s.evaluationClient.GetAssessmentReportV2(ctx, testeeID, assessmentID)
	if err != nil {
		logPersonalityAssessmentError("get personality assessment report failed", err)
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	if err := ensurePersonalityModel(result.Model); err != nil {
		return nil, err
	}
	return toAssessmentReport(result), nil
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

func ensurePersonalityModel(model evaluationapp.ModelIdentityResponse) error {
	if model.Kind != personalityModelKind {
		return errNotPersonalityAssessment
	}
	return nil
}

func toAssessmentDetail(result *evaluationapp.AssessmentDetailV2Response) *AssessmentDetailResponse {
	return &AssessmentDetailResponse{
		ID:                   result.ID,
		OrgID:                result.OrgID,
		TesteeID:             result.TesteeID,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetID:        result.AnswerSheetID,
		Model:                result.Model,
		PrimaryScore:         result.PrimaryScore,
		Level:                result.Level,
		OriginType:           result.OriginType,
		OriginID:             result.OriginID,
		Status:               result.Status,
		SubmittedAt:          result.SubmittedAt,
		InterpretedAt:        result.InterpretedAt,
		FailedAt:             result.FailedAt,
		FailureReason:        result.FailureReason,
	}
}

func toAssessmentSummary(result evaluationapp.AssessmentSummaryV2Response) AssessmentSummaryResponse {
	return AssessmentSummaryResponse{
		ID:                   result.ID,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetID:        result.AnswerSheetID,
		Model:                result.Model,
		PrimaryScore:         result.PrimaryScore,
		Level:                result.Level,
		OriginType:           result.OriginType,
		Status:               result.Status,
		SubmittedAt:          result.SubmittedAt,
		InterpretedAt:        result.InterpretedAt,
	}
}

func toAssessmentReport(result *evaluationapp.AssessmentReportV2Response) *AssessmentReportResponse {
	dimensions := make([]evaluationapp.DimensionInterpretResponse, 0, len(result.Dimensions))
	for _, dim := range result.Dimensions {
		dimensions = append(dimensions, evaluationapp.DimensionInterpretResponse{
			FactorCode:  dim.FactorCode,
			FactorName:  dim.FactorName,
			RawScore:    dim.RawScore,
			MaxScore:    dim.MaxScore,
			RiskLevel:   dim.RiskLevel,
			Description: dim.Description,
			Suggestion:  dim.Suggestion,
		})
	}
	suggestions := make([]evaluationapp.SuggestionResponse, 0, len(result.Suggestions))
	for _, item := range result.Suggestions {
		suggestions = append(suggestions, evaluationapp.SuggestionResponse{
			Category:   item.Category,
			Content:    item.Content,
			FactorCode: item.FactorCode,
		})
	}
	return &AssessmentReportResponse{
		AssessmentID: result.AssessmentID,
		Model:        result.Model,
		PrimaryScore: result.PrimaryScore,
		Level:        result.Level,
		Conclusion:   result.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  suggestions,
		ModelExtra:   result.ModelExtra,
		CreatedAt:    result.CreatedAt,
	}
}

func logPersonalityAssessmentError(message string, err error) {
	if cancelerr.Is(err) {
		log.Debugf("%s: %v", message, err)
		return
	}
	log.Errorf("%s: %v", message, err)
}

func IsNotPersonalityAssessment(err error) bool {
	return errors.Is(err, errNotPersonalityAssessment)
}
