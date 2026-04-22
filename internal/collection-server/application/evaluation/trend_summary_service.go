package evaluation

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

const (
	trendSummaryPageSize      = 100
	trendSummaryTimelineLimit = 6
	trendSummaryFactorLimit   = 3
)

type comparableAssessment struct {
	AssessmentID         string
	ScaleCode            string
	ScaleName            string
	QuestionnaireCode    string
	QuestionnaireVersion string
	SubmittedAt          string
	TotalScore           float64
	RiskLevel            string
	submittedTime        time.Time
}

type factorChangeCandidate struct {
	AssessmentFactorChangeResponse
	riskPriority int
}

// GetAssessmentTrendSummary 获取测评趋势摘要
func (s *QueryService) GetAssessmentTrendSummary(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentTrendSummaryResponse, error) {
	current, err := s.GetMyAssessment(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}

	currentReport, err := s.GetAssessmentReport(ctx, assessmentID)
	if err != nil {
		return nil, err
	}

	comparableItems, err := s.listComparableAssessments(ctx, testeeID, current.ScaleCode, current.QuestionnaireCode, current.QuestionnaireVersion)
	if err != nil {
		return nil, err
	}

	comparableItems = ensureComparableAssessment(comparableItems, current, currentReport)
	sortComparableAssessments(comparableItems)

	currentIndex := findComparableAssessmentIndex(comparableItems, current.ID)
	if currentIndex < 0 {
		currentIndex = len(comparableItems) - 1
	}

	var previousItem *comparableAssessment
	var previousReport *AssessmentReportResponse
	if currentIndex > 0 {
		previousItem = &comparableItems[currentIndex-1]
		previousID, parseErr := strconv.ParseUint(previousItem.AssessmentID, 10, 64)
		if parseErr == nil {
			previousReport, err = s.GetAssessmentReport(ctx, previousID)
			if err != nil {
				return nil, err
			}
		}
	}

	timelineItems := sliceRecentTimeline(comparableItems)
	currentSnapshot := toTrendSnapshot(current, currentReport)
	if match := findComparableAssessmentByID(comparableItems, current.ID); match != nil {
		if currentSnapshot.SubmittedAt == "" {
			currentSnapshot.SubmittedAt = match.SubmittedAt
		}
		if currentSnapshot.ScaleName == "" {
			currentSnapshot.ScaleName = match.ScaleName
		}
	}

	var previousSnapshot *AssessmentTrendSnapshotResponse
	if previousItem != nil {
		previousSnapshot = &AssessmentTrendSnapshotResponse{
			AssessmentID:         previousItem.AssessmentID,
			ScaleCode:            previousItem.ScaleCode,
			ScaleName:            previousItem.ScaleName,
			QuestionnaireVersion: previousItem.QuestionnaireVersion,
			SubmittedAt:          previousItem.SubmittedAt,
			TotalScore:           previousItem.TotalScore,
			RiskLevel:            previousItem.RiskLevel,
		}
	}

	factorChanges := buildFactorChanges(currentReport, previousReport)
	selectedFactors := pickTrendFactors(factorChanges)
	factorTrends, err := s.buildFactorTrends(ctx, testeeID, selectedFactors, comparableItems)
	if err != nil {
		return nil, err
	}
	comparableCount, err := safeconv.IntToInt32(len(comparableItems))
	if err != nil {
		return nil, err
	}

	return &AssessmentTrendSummaryResponse{
		Current:       currentSnapshot,
		Previous:      previousSnapshot,
		Timeline:      toTrendTimeline(timelineItems),
		FactorChanges: toFactorChangeResponses(selectedFactors),
		FactorTrends:  factorTrends,
		Meta: AssessmentTrendMetaResponse{
			ComparableCount:    comparableCount,
			HasMultipleFillers: false,
			DisplayMode:        "same_scale_same_version",
			Note:               buildTrendNote(len(comparableItems), previousItem != nil),
		},
	}, nil
}

func (s *QueryService) listComparableAssessments(
	ctx context.Context,
	testeeID uint64,
	scaleCode string,
	questionnaireCode string,
	questionnaireVersion string,
) ([]comparableAssessment, error) {
	page := int32(1)
	items := make([]comparableAssessment, 0)

	for {
		result, err := s.ListMyAssessments(ctx, testeeID, &ListAssessmentsRequest{
			Status:    "interpreted",
			Page:      page,
			PageSize:  trendSummaryPageSize,
			ScaleCode: scaleCode,
		})
		if err != nil {
			return nil, err
		}
		if result == nil || len(result.Items) == 0 {
			break
		}

		for _, item := range result.Items {
			if questionnaireCode != "" && item.QuestionnaireCode != questionnaireCode {
				continue
			}
			if questionnaireVersion != "" && item.QuestionnaireVersion != questionnaireVersion {
				continue
			}

			submittedAt := firstNonEmpty(item.SubmittedAt, item.InterpretedAt, item.CreatedAt)
			items = append(items, comparableAssessment{
				AssessmentID:         item.ID,
				ScaleCode:            item.ScaleCode,
				ScaleName:            item.ScaleName,
				QuestionnaireCode:    item.QuestionnaireCode,
				QuestionnaireVersion: item.QuestionnaireVersion,
				SubmittedAt:          submittedAt,
				TotalScore:           item.TotalScore,
				RiskLevel:            item.RiskLevel,
				submittedTime:        parseTrendTime(submittedAt),
			})
		}

		if page >= result.TotalPages {
			break
		}
		page++
	}

	return items, nil
}

func ensureComparableAssessment(
	items []comparableAssessment,
	current *AssessmentDetailResponse,
	currentReport *AssessmentReportResponse,
) []comparableAssessment {
	if findComparableAssessmentIndex(items, current.ID) >= 0 {
		return items
	}

	submittedAt := firstNonEmpty(
		current.SubmittedAt,
		current.InterpretedAt,
		current.CreatedAt,
		valueFromReport(currentReport, func(r *AssessmentReportResponse) string { return r.CreatedAt }),
	)

	items = append(items, comparableAssessment{
		AssessmentID:         current.ID,
		ScaleCode:            current.ScaleCode,
		ScaleName:            firstNonEmpty(current.ScaleName, valueFromReport(currentReport, func(r *AssessmentReportResponse) string { return r.ScaleName })),
		QuestionnaireCode:    current.QuestionnaireCode,
		QuestionnaireVersion: current.QuestionnaireVersion,
		SubmittedAt:          submittedAt,
		TotalScore:           current.TotalScore,
		RiskLevel:            current.RiskLevel,
		submittedTime:        parseTrendTime(submittedAt),
	})
	return items
}

func sortComparableAssessments(items []comparableAssessment) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].submittedTime.Equal(items[j].submittedTime) {
			return items[i].AssessmentID < items[j].AssessmentID
		}
		return items[i].submittedTime.Before(items[j].submittedTime)
	})
}

func findComparableAssessmentIndex(items []comparableAssessment, assessmentID string) int {
	for i, item := range items {
		if item.AssessmentID == assessmentID {
			return i
		}
	}
	return -1
}

func findComparableAssessmentByID(items []comparableAssessment, assessmentID string) *comparableAssessment {
	for i := range items {
		if items[i].AssessmentID == assessmentID {
			return &items[i]
		}
	}
	return nil
}

func sliceRecentTimeline(items []comparableAssessment) []comparableAssessment {
	if len(items) <= trendSummaryTimelineLimit {
		return items
	}
	return items[len(items)-trendSummaryTimelineLimit:]
}

func toTrendSnapshot(current *AssessmentDetailResponse, report *AssessmentReportResponse) *AssessmentTrendSnapshotResponse {
	if current == nil {
		return nil
	}
	return &AssessmentTrendSnapshotResponse{
		AssessmentID:         current.ID,
		ScaleCode:            current.ScaleCode,
		ScaleName:            firstNonEmpty(current.ScaleName, valueFromReport(report, func(r *AssessmentReportResponse) string { return r.ScaleName })),
		QuestionnaireVersion: current.QuestionnaireVersion,
		SubmittedAt:          firstNonEmpty(current.SubmittedAt, current.InterpretedAt, current.CreatedAt, valueFromReport(report, func(r *AssessmentReportResponse) string { return r.CreatedAt })),
		TotalScore:           current.TotalScore,
		RiskLevel:            current.RiskLevel,
	}
}

func toTrendTimeline(items []comparableAssessment) []AssessmentTrendTimelinePointResponse {
	if len(items) == 0 {
		return nil
	}

	result := make([]AssessmentTrendTimelinePointResponse, 0, len(items))
	for _, item := range items {
		result = append(result, AssessmentTrendTimelinePointResponse{
			AssessmentID: item.AssessmentID,
			SubmittedAt:  item.SubmittedAt,
			TotalScore:   item.TotalScore,
			RiskLevel:    item.RiskLevel,
		})
	}
	return result
}

func buildFactorChanges(currentReport, previousReport *AssessmentReportResponse) []factorChangeCandidate {
	if currentReport == nil || previousReport == nil || len(currentReport.Dimensions) == 0 {
		return nil
	}

	previousMap := make(map[string]DimensionInterpretResponse, len(previousReport.Dimensions))
	for _, factor := range previousReport.Dimensions {
		previousMap[factor.FactorCode] = factor
	}

	highPriority := make([]factorChangeCandidate, 0)
	others := make([]factorChangeCandidate, 0)

	for _, factor := range currentReport.Dimensions {
		previousFactor, ok := previousMap[factor.FactorCode]
		if !ok {
			continue
		}

		candidate := factorChangeCandidate{
			AssessmentFactorChangeResponse: AssessmentFactorChangeResponse{
				FactorCode:    factor.FactorCode,
				FactorName:    factor.FactorName,
				CurrentScore:  factor.RawScore,
				PreviousScore: previousFactor.RawScore,
				Delta:         factor.RawScore - previousFactor.RawScore,
				RiskLevel:     factor.RiskLevel,
			},
			riskPriority: normalizeRiskPriority(factor.RiskLevel),
		}

		if candidate.riskPriority >= 2 {
			highPriority = append(highPriority, candidate)
			continue
		}
		others = append(others, candidate)
	}

	sortFactorCandidates(highPriority)
	sortFactorCandidates(others)

	result := make([]factorChangeCandidate, 0, trendSummaryFactorLimit)
	result = append(result, highPriority...)
	if len(result) < trendSummaryFactorLimit {
		result = append(result, others...)
	}
	if len(result) > trendSummaryFactorLimit {
		result = result[:trendSummaryFactorLimit]
	}

	return result
}

func sortFactorCandidates(items []factorChangeCandidate) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].riskPriority != items[j].riskPriority {
			return items[i].riskPriority > items[j].riskPriority
		}
		leftDelta := absFloat(items[i].Delta)
		rightDelta := absFloat(items[j].Delta)
		if leftDelta != rightDelta {
			return leftDelta > rightDelta
		}
		return items[i].CurrentScore > items[j].CurrentScore
	})
}

func pickTrendFactors(items []factorChangeCandidate) []factorChangeCandidate {
	if len(items) <= trendSummaryFactorLimit {
		return items
	}
	return items[:trendSummaryFactorLimit]
}

func toFactorChangeResponses(items []factorChangeCandidate) []AssessmentFactorChangeResponse {
	if len(items) == 0 {
		return nil
	}
	result := make([]AssessmentFactorChangeResponse, 0, len(items))
	for _, item := range items {
		result = append(result, item.AssessmentFactorChangeResponse)
	}
	return result
}

func (s *QueryService) buildFactorTrends(
	ctx context.Context,
	testeeID uint64,
	factors []factorChangeCandidate,
	comparableItems []comparableAssessment,
) ([]AssessmentFactorTrendResponse, error) {
	if len(factors) == 0 || len(comparableItems) == 0 {
		return nil, nil
	}

	allowed := make(map[string]comparableAssessment, len(comparableItems))
	for _, item := range comparableItems {
		allowed[item.AssessmentID] = item
	}

	result := make([]AssessmentFactorTrendResponse, 0, len(factors))
	for _, factor := range factors {
		points, err := s.evaluationClient.GetFactorTrend(ctx, testeeID, factor.FactorCode, 50)
		if err != nil {
			return nil, err
		}

		trendPoints := make([]AssessmentFactorTrendPointResponse, 0, len(points))
		for _, point := range points {
			assessmentID := strconv.FormatUint(point.AssessmentID, 10)
			allowedItem, ok := allowed[assessmentID]
			if !ok {
				continue
			}

			trendPoints = append(trendPoints, AssessmentFactorTrendPointResponse{
				AssessmentID: assessmentID,
				SubmittedAt:  firstNonEmpty(allowedItem.SubmittedAt, point.CreatedAt),
				Score:        point.Score,
				RiskLevel:    point.RiskLevel,
			})
		}

		sort.SliceStable(trendPoints, func(i, j int) bool {
			left := parseTrendTime(trendPoints[i].SubmittedAt)
			right := parseTrendTime(trendPoints[j].SubmittedAt)
			if left.Equal(right) {
				return trendPoints[i].AssessmentID < trendPoints[j].AssessmentID
			}
			return left.Before(right)
		})

		result = append(result, AssessmentFactorTrendResponse{
			FactorCode: factor.FactorCode,
			FactorName: factor.FactorName,
			Points:     trendPoints,
		})
	}

	return result, nil
}

func buildTrendNote(comparableCount int, hasPrevious bool) string {
	if comparableCount < 2 {
		return "完成 2 次同量表测评后可查看变化趋势"
	}
	if !hasPrevious {
		return "当前报告之前暂无可比较记录，已展示同量表同版本的历史趋势"
	}
	return "仅展示同一量表、同一版本的历史结果"
}

func normalizeRiskPriority(level string) int {
	switch strings.ToLower(level) {
	case "high", "high_risk", "severe", "critical":
		return 3
	case "medium", "mid", "moderate", "medium_risk":
		return 2
	case "low", "low_risk", "mild":
		return 1
	default:
		return 0
	}
}

func parseTrendTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func valueFromReport(report *AssessmentReportResponse, fn func(*AssessmentReportResponse) string) string {
	if report == nil {
		return ""
	}
	return fn(report)
}
