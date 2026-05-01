package testee

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
)

func TestScaleAnalysisQueryGroupsAndSortsInterpretedAssessments(t *testing.T) {
	submittedAt := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	interpretedLate := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)
	interpretedEarly := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	score := 18.5
	risk := "high"
	scaleA := "scale-a"
	scaleB := "scale-b"
	scaleAID := uint64(101)
	scaleBID := uint64(102)
	scaleAName := "Scale A"
	scaleBName := "Scale B"
	assessmentManagement := &scaleAnalysisAssessmentManagementStub{
		items: []*assessmentApp.AssessmentResult{
			{ID: 3, Status: "interpreted", MedicalScaleID: &scaleBID, MedicalScaleCode: &scaleB, MedicalScaleName: &scaleBName, InterpretedAt: &interpretedLate, SubmittedAt: &submittedAt, TotalScore: &score, RiskLevel: &risk},
			{ID: 9, Status: "submitted", MedicalScaleID: &scaleAID, MedicalScaleCode: &scaleA, MedicalScaleName: &scaleAName, InterpretedAt: &interpretedEarly},
			{ID: 4, Status: "interpreted", MedicalScaleID: &scaleAID, MedicalScaleCode: &scaleA, MedicalScaleName: &scaleAName, InterpretedAt: &interpretedEarly},
			{ID: 5, Status: "interpreted"},
		},
	}
	scoreQuery := &scaleAnalysisScoreQueryStub{
		byAssessmentID: map[uint64]*assessmentApp.ScoreResult{
			3: {
				AssessmentID: 3,
				FactorScores: []assessmentApp.FactorScoreResult{
					{FactorCode: "f1", FactorName: "Factor 1", RawScore: 7.5, RiskLevel: "medium"},
				},
			},
			4: {AssessmentID: 4},
		},
	}
	service := NewScaleAnalysisQueryService(assessmentManagement, scoreQuery)

	result, err := service.GetScaleAnalysis(context.Background(), ScaleAnalysisQueryDTO{OrgID: 1, TesteeID: 20})
	if err != nil {
		t.Fatalf("GetScaleAnalysis returned error: %v", err)
	}

	if assessmentManagement.listCalls != 1 {
		t.Fatalf("expected assessment list to be called once, got %d", assessmentManagement.listCalls)
	}
	if assessmentManagement.lastDTO.OrgID != 1 || assessmentManagement.lastDTO.Conditions["testee_id"] != "20" {
		t.Fatalf("unexpected assessment list dto: %+v", assessmentManagement.lastDTO)
	}
	if len(result.Scales) != 2 {
		t.Fatalf("expected two scale groups, got %d", len(result.Scales))
	}
	if result.Scales[0].ScaleCode != "scale-a" || result.Scales[1].ScaleCode != "scale-b" {
		t.Fatalf("expected scales sorted by code, got %+v", result.Scales)
	}
	if len(result.Scales[0].Tests) != 1 || result.Scales[0].Tests[0].AssessmentID != 4 {
		t.Fatalf("expected interpreted scale-a assessment only, got %+v", result.Scales[0].Tests)
	}
	if got := result.Scales[1].Tests[0].TestDate; !got.Equal(interpretedLate) {
		t.Fatalf("expected interpreted_at to win over submitted_at, got %v", got)
	}
	if len(result.Scales[1].Tests[0].Factors) != 1 || result.Scales[1].Tests[0].Factors[0].FactorCode != "f1" {
		t.Fatalf("expected factor scores to be mapped, got %+v", result.Scales[1].Tests[0].Factors)
	}
}

func TestScaleAnalysisQuerySortsTestsByDateAndFallsBackOnScoreError(t *testing.T) {
	older := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	scaleCode := "scale-a"
	assessmentManagement := &scaleAnalysisAssessmentManagementStub{
		items: []*assessmentApp.AssessmentResult{
			{ID: 2, Status: "interpreted", MedicalScaleCode: &scaleCode, SubmittedAt: &newer},
			{ID: 1, Status: "interpreted", MedicalScaleCode: &scaleCode, SubmittedAt: &older},
		},
	}
	scoreQuery := &scaleAnalysisScoreQueryStub{
		errByAssessmentID: map[uint64]error{
			1: stderrors.New("score query failed"),
			2: stderrors.New("score query failed"),
		},
	}
	service := NewScaleAnalysisQueryService(assessmentManagement, scoreQuery)

	result, err := service.GetScaleAnalysis(context.Background(), ScaleAnalysisQueryDTO{OrgID: 1, TesteeID: 20})
	if err != nil {
		t.Fatalf("GetScaleAnalysis returned error: %v", err)
	}

	if len(result.Scales) != 1 || len(result.Scales[0].Tests) != 2 {
		t.Fatalf("expected one scale with two tests, got %+v", result.Scales)
	}
	if result.Scales[0].Tests[0].AssessmentID != 1 || result.Scales[0].Tests[1].AssessmentID != 2 {
		t.Fatalf("expected tests sorted by date ascending, got %+v", result.Scales[0].Tests)
	}
	for _, item := range result.Scales[0].Tests {
		if len(item.Factors) != 0 {
			t.Fatalf("expected score query error to fall back to empty factors, got %+v", item.Factors)
		}
	}
}

type scaleAnalysisAssessmentManagementStub struct {
	items     []*assessmentApp.AssessmentResult
	listCalls int
	lastDTO   assessmentApp.ListAssessmentsDTO
}

func (s *scaleAnalysisAssessmentManagementStub) GetByID(context.Context, uint64) (*assessmentApp.AssessmentResult, error) {
	return nil, nil
}

func (s *scaleAnalysisAssessmentManagementStub) List(_ context.Context, dto assessmentApp.ListAssessmentsDTO) (*assessmentApp.AssessmentListResult, error) {
	s.listCalls++
	s.lastDTO = dto
	return &assessmentApp.AssessmentListResult{Items: s.items, Total: len(s.items), Page: dto.Page, PageSize: dto.PageSize}, nil
}

func (s *scaleAnalysisAssessmentManagementStub) Retry(context.Context, int64, uint64) (*assessmentApp.AssessmentResult, error) {
	return nil, nil
}

type scaleAnalysisScoreQueryStub struct {
	byAssessmentID    map[uint64]*assessmentApp.ScoreResult
	errByAssessmentID map[uint64]error
}

func (s *scaleAnalysisScoreQueryStub) GetByAssessmentID(_ context.Context, assessmentID uint64) (*assessmentApp.ScoreResult, error) {
	if err := s.errByAssessmentID[assessmentID]; err != nil {
		return nil, err
	}
	if result := s.byAssessmentID[assessmentID]; result != nil {
		return result, nil
	}
	return &assessmentApp.ScoreResult{AssessmentID: assessmentID}, nil
}

func (s *scaleAnalysisScoreQueryStub) GetFactorTrend(context.Context, assessmentApp.GetFactorTrendDTO) (*assessmentApp.FactorTrendResult, error) {
	return nil, nil
}

func (s *scaleAnalysisScoreQueryStub) GetHighRiskFactors(context.Context, uint64) (*assessmentApp.HighRiskFactorsResult, error) {
	return nil, nil
}
