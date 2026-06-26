package personalityassessment

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/pkg/cancelerr"
)

const personalityModelKind = "personality"

var errNotPersonalityAssessment = errors.New("assessment is not a personality evaluation")

type QueryService struct {
	evaluationClient *grpcclient.EvaluationClient
	waitReport       *reportwait.Service
}

func NewQueryService(evaluationClient *grpcclient.EvaluationClient, waitReport *reportwait.Service) *QueryService {
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

func (s *QueryService) GetReport(ctx context.Context, assessmentID uint64) (*AssessmentReportResponse, error) {
	result, err := s.evaluationClient.GetAssessmentReportV2(ctx, assessmentID)
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

func (s *QueryService) WaitReport(ctx context.Context, testeeID, assessmentID uint64, timeout time.Duration) (*AssessmentStatusResponse, error) {
	if s.waitReport == nil {
		return nil, fmt.Errorf("wait report service is not configured")
	}
	status, err := s.waitReport.Wait(ctx, testeeID, assessmentID, timeout)
	if err != nil {
		return nil, err
	}
	resp := &AssessmentStatusResponse{
		Status:          status.Status,
		Stage:           status.Stage,
		Message:         status.Message,
		Reason:          status.Reason,
		NextPollAfterMs: status.NextPollAfterMs,
		UpdatedAt:       status.UpdatedAt,
	}
	if status.Status == "interpreted" {
		if detail, err := s.Get(ctx, testeeID, assessmentID); err == nil && detail != nil {
			model := detail.Model
			resp.Model = &model
			resp.Level = detail.Level
		}
	}
	return resp, nil
}

func ensurePersonalityModel(model grpcclient.ModelIdentityOutput) error {
	if model.Kind != personalityModelKind {
		return errNotPersonalityAssessment
	}
	return nil
}

func toAssessmentDetail(result *grpcclient.AssessmentDetailV2Output) *AssessmentDetailResponse {
	return &AssessmentDetailResponse{
		ID:                   strconv.FormatUint(result.ID, 10),
		OrgID:                strconv.FormatUint(result.OrgID, 10),
		TesteeID:             strconv.FormatUint(result.TesteeID, 10),
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetID:        strconv.FormatUint(result.AnswerSheetID, 10),
		Model:                toModelIdentity(result.Model),
		PrimaryScore:         toScoreValue(result.PrimaryScore),
		Level:                toResultLevel(result.Level),
		OriginType:           result.OriginType,
		OriginID:             result.OriginID,
		Status:               result.Status,
		SubmittedAt:          result.SubmittedAt,
		InterpretedAt:        result.InterpretedAt,
		FailedAt:             result.FailedAt,
		FailureReason:        result.FailureReason,
	}
}

func toAssessmentSummary(result grpcclient.AssessmentSummaryV2Output) AssessmentSummaryResponse {
	return AssessmentSummaryResponse{
		ID:                   strconv.FormatUint(result.ID, 10),
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetID:        strconv.FormatUint(result.AnswerSheetID, 10),
		Model:                toModelIdentity(result.Model),
		PrimaryScore:         toScoreValue(result.PrimaryScore),
		Level:                toResultLevel(result.Level),
		OriginType:           result.OriginType,
		Status:               result.Status,
		SubmittedAt:          result.SubmittedAt,
		InterpretedAt:        result.InterpretedAt,
	}
}

func toAssessmentReport(result *grpcclient.AssessmentReportV2Output) *AssessmentReportResponse {
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
		AssessmentID: strconv.FormatUint(result.AssessmentID, 10),
		Model:        toModelIdentity(result.Model),
		PrimaryScore: toScoreValue(result.PrimaryScore),
		Level:        toResultLevel(result.Level),
		Conclusion:   result.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  suggestions,
		ModelExtra:   toModelExtra(result.ModelExtra),
		CreatedAt:    result.CreatedAt,
	}
}

func toModelIdentity(model grpcclient.ModelIdentityOutput) ModelIdentityResponse {
	return ModelIdentityResponse{
		Kind:      model.Kind,
		SubKind:   model.SubKind,
		Algorithm: model.Algorithm,
		Code:      model.Code,
		Version:   model.Version,
		Title:     model.Title,
	}
}

func toScoreValue(score *grpcclient.ScoreValueOutput) *ScoreValueResponse {
	if score == nil {
		return nil
	}
	return &ScoreValueResponse{
		Kind:  score.Kind,
		Value: score.Value,
		Label: score.Label,
		Max:   score.Max,
	}
}

func toResultLevel(level *grpcclient.ResultLevelOutput) *ResultLevelResponse {
	if level == nil {
		return nil
	}
	return &ResultLevelResponse{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}

func toModelExtra(extra *grpcclient.ModelExtraOutput) *ModelExtraResponse {
	if extra == nil {
		return nil
	}
	resp := &ModelExtraResponse{
		Kind:           extra.Kind,
		TypeCode:       extra.TypeCode,
		TypeName:       extra.TypeName,
		OneLiner:       extra.OneLiner,
		ImageURL:       extra.ImageURL,
		MatchPercent:   extra.MatchPercent,
		IsSpecial:      extra.IsSpecial,
		SpecialTrigger: extra.SpecialTrigger,
		Commentary:     extra.Commentary,
	}
	if extra.Rarity != nil {
		resp.Rarity = &ModelRarityResponse{
			Percent: extra.Rarity.Percent,
			Label:   extra.Rarity.Label,
			OneInX:  extra.Rarity.OneInX,
		}
	}
	return resp
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
