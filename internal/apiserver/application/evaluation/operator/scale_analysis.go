package operator

import (
	"context"
	"sort"
	"time"
)

type ScaleAnalysis struct {
	TesteeID uint64
	Scales   []ScaleTrend
}
type ScaleTrend struct {
	ScaleID, ScaleCode, ScaleName string
	Tests                         []ScaleTest
}
type ScaleTest struct {
	AssessmentID      uint64
	TestDate          time.Time
	TotalScore        float64
	RiskLevel, Result string
	Factors           []ScaleFactor
}
type ScaleFactor struct {
	FactorCode, FactorName string
	RawScore               float64
	RiskLevel              string
}
type ScaleAnalysisService interface {
	GetScaleAnalysis(context.Context, Actor, uint64) (*ScaleAnalysis, error)
}
type scaleAnalysisService struct{ queries QueryService }

func NewScaleAnalysisService(queries QueryService) ScaleAnalysisService {
	return &scaleAnalysisService{queries: queries}
}

func (s *scaleAnalysisService) GetScaleAnalysis(ctx context.Context, actor Actor, testeeID uint64) (*ScaleAnalysis, error) {
	list, err := s.queries.ListAssessments(ctx, actor, ListQuery{Page: 1, PageSize: 100, TesteeID: &testeeID})
	if err != nil {
		return nil, err
	}
	grouped := map[string]*ScaleTrend{}
	for _, assessment := range list.Items {
		if assessment == nil || assessment.Status != "evaluated" || assessment.ModelKind == nil || *assessment.ModelKind != "scale" || assessment.ModelCode == nil {
			continue
		}
		code := *assessment.ModelCode
		trend := grouped[code]
		if trend == nil {
			trend = &ScaleTrend{ScaleID: code, ScaleCode: code, ScaleName: stringPointerValue(assessment.ModelTitle), Tests: []ScaleTest{}}
			grouped[code] = trend
		}
		test := ScaleTest{AssessmentID: assessment.ID, TestDate: assessmentDate(assessment), TotalScore: floatPointerValue(assessment.TotalScore), RiskLevel: stringPointerValue(assessment.RiskLevel), Factors: []ScaleFactor{}}
		if score, scoreErr := s.queries.GetScores(ctx, actor, assessment.ID); scoreErr == nil && score != nil {
			for _, factor := range score.FactorScores {
				test.Factors = append(test.Factors, ScaleFactor{FactorCode: factor.FactorCode, FactorName: factor.FactorName, RawScore: factor.RawScore, RiskLevel: factor.RiskLevel})
			}
		}
		trend.Tests = append(trend.Tests, test)
	}
	result := &ScaleAnalysis{TesteeID: testeeID, Scales: make([]ScaleTrend, 0, len(grouped))}
	for _, trend := range grouped {
		sort.Slice(trend.Tests, func(i, j int) bool { return trend.Tests[i].TestDate.Before(trend.Tests[j].TestDate) })
		result.Scales = append(result.Scales, *trend)
	}
	sort.Slice(result.Scales, func(i, j int) bool { return result.Scales[i].ScaleCode < result.Scales[j].ScaleCode })
	return result, nil
}
func assessmentDate(a *Assessment) time.Time {
	if a.EvaluatedAt != nil {
		return *a.EvaluatedAt
	}
	if a.SubmittedAt != nil {
		return *a.SubmittedAt
	}
	return time.Time{}
}
func stringPointerValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func floatPointerValue(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
