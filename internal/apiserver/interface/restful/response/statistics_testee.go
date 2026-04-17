package response

import domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"

// TesteeStatisticsResponse 受试者统计响应。
type TesteeStatisticsResponse struct {
	OrgID                int64            `json:"org_id"`
	TesteeID             uint64           `json:"testee_id"`
	TotalAssessments     int64            `json:"total_assessments"`
	CompletedAssessments int64            `json:"completed_assessments"`
	PendingAssessments   int64            `json:"pending_assessments"`
	RiskDistribution     map[string]int64 `json:"risk_distribution"`
	LastAssessmentDate   *string          `json:"last_assessment_date,omitempty"`
	FirstAssessmentDate  *string          `json:"first_assessment_date,omitempty"`
}

func NewTesteeStatisticsResponse(stats *domainStatistics.TesteeStatistics) *TesteeStatisticsResponse {
	if stats == nil {
		return nil
	}

	return &TesteeStatisticsResponse{
		OrgID:                stats.OrgID,
		TesteeID:             stats.TesteeID,
		TotalAssessments:     stats.TotalAssessments,
		CompletedAssessments: stats.CompletedAssessments,
		PendingAssessments:   stats.PendingAssessments,
		RiskDistribution:     stats.RiskDistribution,
		LastAssessmentDate:   FormatDateTimePtr(stats.LastAssessmentDate),
		FirstAssessmentDate:  FormatDateTimePtr(stats.FirstAssessmentDate),
	}
}
