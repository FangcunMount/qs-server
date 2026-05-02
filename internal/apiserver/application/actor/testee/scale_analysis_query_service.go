package testee

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type scaleAnalysisQueryService struct {
	assessmentManagement assessmentApp.AssessmentManagementService
	scoreQuery           assessmentApp.ScoreQueryService
}

func NewScaleAnalysisQueryService(
	assessmentManagement assessmentApp.AssessmentManagementService,
	scoreQuery assessmentApp.ScoreQueryService,
) ScaleAnalysisQueryService {
	return &scaleAnalysisQueryService{
		assessmentManagement: assessmentManagement,
		scoreQuery:           scoreQuery,
	}
}

func (s *scaleAnalysisQueryService) GetScaleAnalysis(ctx context.Context, dto ScaleAnalysisQueryDTO) (*ScaleAnalysisQueryResult, error) {
	orgScope, err := safeconv.Int64ToUint64(dto.OrgID)
	if err != nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "org scope exceeds uint64")
	}
	if s.assessmentManagement == nil {
		return &ScaleAnalysisQueryResult{TesteeID: dto.TesteeID, Scales: []ScaleTrendQueryResult{}}, nil
	}

	testeeID := dto.TesteeID
	assessmentList, err := s.assessmentManagement.List(ctx, assessmentApp.ListAssessmentsDTO{
		OrgID:    orgScope,
		Page:     1,
		PageSize: 1000,
		TesteeID: &testeeID,
	})
	if err != nil {
		return nil, err
	}

	scaleMap := make(map[string]*ScaleTrendQueryResult)
	for _, assessment := range assessmentList.Items {
		if !isScaleAnalysisAssessment(assessment) {
			continue
		}
		scaleTrend := ensureScaleAnalysisTrend(scaleMap, assessment)
		scaleTrend.Tests = append(scaleTrend.Tests, s.buildScaleAnalysisTest(ctx, assessment))
	}
	return &ScaleAnalysisQueryResult{
		TesteeID: dto.TesteeID,
		Scales:   flattenScaleAnalysisTrends(scaleMap),
	}, nil
}

func isScaleAnalysisAssessment(assessment *assessmentApp.AssessmentResult) bool {
	return assessment != nil && assessment.Status == "interpreted" && assessment.MedicalScaleCode != nil
}

func ensureScaleAnalysisTrend(scaleMap map[string]*ScaleTrendQueryResult, assessment *assessmentApp.AssessmentResult) *ScaleTrendQueryResult {
	scaleCode := *assessment.MedicalScaleCode
	if existing, ok := scaleMap[scaleCode]; ok {
		return existing
	}
	scaleTrend := &ScaleTrendQueryResult{
		ScaleID:   scaleAnalysisScaleID(assessment),
		ScaleCode: scaleCode,
		ScaleName: scaleAnalysisScaleName(assessment),
		Tests:     []ScaleTestQueryResult{},
	}
	scaleMap[scaleCode] = scaleTrend
	return scaleTrend
}

func scaleAnalysisScaleID(assessment *assessmentApp.AssessmentResult) string {
	if assessment.MedicalScaleID == nil {
		return ""
	}
	return strconv.FormatUint(*assessment.MedicalScaleID, 10)
}

func scaleAnalysisScaleName(assessment *assessmentApp.AssessmentResult) string {
	if assessment.MedicalScaleName == nil {
		return ""
	}
	return *assessment.MedicalScaleName
}

func (s *scaleAnalysisQueryService) buildScaleAnalysisTest(ctx context.Context, assessment *assessmentApp.AssessmentResult) ScaleTestQueryResult {
	totalScore := 0.0
	if assessment.TotalScore != nil {
		totalScore = *assessment.TotalScore
	}
	riskLevel := ""
	if assessment.RiskLevel != nil {
		riskLevel = *assessment.RiskLevel
	}
	return ScaleTestQueryResult{
		AssessmentID: assessment.ID,
		TestDate:     scaleAnalysisTestDate(assessment),
		TotalScore:   totalScore,
		RiskLevel:    riskLevel,
		Result:       "",
		Factors:      s.loadScaleAnalysisFactors(ctx, assessment.ID),
	}
}

func (s *scaleAnalysisQueryService) loadScaleAnalysisFactors(ctx context.Context, assessmentID uint64) []ScaleFactorQueryResult {
	if s.scoreQuery == nil {
		return []ScaleFactorQueryResult{}
	}
	scoreResult, err := s.scoreQuery.GetByAssessmentID(ctx, assessmentID)
	if err != nil || scoreResult == nil {
		return []ScaleFactorQueryResult{}
	}
	factors := make([]ScaleFactorQueryResult, 0, len(scoreResult.FactorScores))
	for _, factorScore := range scoreResult.FactorScores {
		factors = append(factors, ScaleFactorQueryResult{
			FactorCode: factorScore.FactorCode,
			FactorName: factorScore.FactorName,
			RawScore:   factorScore.RawScore,
			RiskLevel:  factorScore.RiskLevel,
		})
	}
	return factors
}

func scaleAnalysisTestDate(assessment *assessmentApp.AssessmentResult) time.Time {
	if assessment.InterpretedAt != nil {
		return *assessment.InterpretedAt
	}
	if assessment.SubmittedAt != nil {
		return *assessment.SubmittedAt
	}
	return time.Time{}
}

func flattenScaleAnalysisTrends(scaleMap map[string]*ScaleTrendQueryResult) []ScaleTrendQueryResult {
	scales := make([]ScaleTrendQueryResult, 0, len(scaleMap))
	for _, scaleTrend := range scaleMap {
		sort.Slice(scaleTrend.Tests, func(i, j int) bool {
			return scaleTrend.Tests[i].TestDate.Before(scaleTrend.Tests[j].TestDate)
		})
		scales = append(scales, *scaleTrend)
	}
	sort.Slice(scales, func(i, j int) bool {
		return scales[i].ScaleCode < scales[j].ScaleCode
	})
	return scales
}
