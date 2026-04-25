package response

import (
	"testing"
	"time"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

func TestNewAssessmentResponseAddsLabelsAndFormatsTimes(t *testing.T) {
	submittedAt := time.Date(2026, 4, 17, 13, 25, 27, 0, time.Local)
	riskLevel := "high"
	result := &assessmentApp.AssessmentResult{
		ID:                   1,
		OrgID:                2,
		TesteeID:             3,
		QuestionnaireCode:    "q",
		QuestionnaireVersion: "v1",
		AnswerSheetID:        4,
		OriginType:           "plan",
		Status:               "interpreted",
		RiskLevel:            &riskLevel,
		SubmittedAt:          &submittedAt,
	}

	resp := NewAssessmentResponse(result)
	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.OriginTypeLabel != "计划测评" {
		t.Fatalf("origin_type_label = %q, want %q", resp.OriginTypeLabel, "计划测评")
	}
	if resp.StatusLabel != "已解读" {
		t.Fatalf("status_label = %q, want %q", resp.StatusLabel, "已解读")
	}
	if resp.RiskLevelLabel != "高风险" {
		t.Fatalf("risk_level_label = %q, want %q", resp.RiskLevelLabel, "高风险")
	}
	if resp.SubmittedAt == nil || *resp.SubmittedAt != "2026-04-17 13:25:27" {
		t.Fatalf("submitted_at = %#v, want %q", resp.SubmittedAt, "2026-04-17 13:25:27")
	}
}

func TestNewPeriodicStatsResponseFormatsDatesAndLabels(t *testing.T) {
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local)
	completedAt := time.Date(2026, 4, 17, 9, 30, 0, 0, time.Local)
	dueDate := time.Date(2026, 4, 20, 0, 0, 0, 0, time.Local)
	plannedAt := time.Date(2026, 4, 16, 8, 0, 0, 0, time.Local)
	assessmentID := "1001"

	resp := NewPeriodicStatsResponse(&domainStatistics.TesteePeriodicStatisticsResponse{
		Projects: []domainStatistics.TesteePeriodicProjectStatistics{
			{
				ProjectID:   "p1",
				ProjectName: "周期计划",
				ScaleName:   "CBCL",
				StartDate:   &start,
				Tasks: []domainStatistics.TesteePeriodicTaskStatistics{
					{
						Week:         1,
						Status:       "completed",
						CompletedAt:  &completedAt,
						PlannedAt:    &plannedAt,
						DueDate:      &dueDate,
						AssessmentID: &assessmentID,
					},
				},
			},
		},
		TotalProjects:  1,
		ActiveProjects: 1,
	})

	if len(resp.Projects) != 1 {
		t.Fatalf("projects len = %d, want 1", len(resp.Projects))
	}
	project := resp.Projects[0]
	if project.StartDate == nil || *project.StartDate != "2026-04-01" {
		t.Fatalf("start_date = %#v, want %q", project.StartDate, "2026-04-01")
	}
	task := project.Tasks[0]
	if task.StatusLabel != "已完成" {
		t.Fatalf("status_label = %q, want %q", task.StatusLabel, "已完成")
	}
	if task.CompletedAt == nil || *task.CompletedAt != "2026-04-17 09:30:00" {
		t.Fatalf("completed_at = %#v, want %q", task.CompletedAt, "2026-04-17 09:30:00")
	}
	if task.PlannedAt == nil || *task.PlannedAt != "2026-04-16 08:00:00" {
		t.Fatalf("planned_at = %#v, want %q", task.PlannedAt, "2026-04-16 08:00:00")
	}
	if task.DueDate == nil || *task.DueDate != "2026-04-20" {
		t.Fatalf("due_date = %#v, want %q", task.DueDate, "2026-04-20")
	}
}

func TestLabelTagsUsesDisplayNames(t *testing.T) {
	got := LabelTags([]string{"risk_high", "custom_tag"})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0] != "高风险" {
		t.Fatalf("got[0] = %q, want %q", got[0], "高风险")
	}
	if got[1] != "custom_tag" {
		t.Fatalf("got[1] = %q, want %q", got[1], "custom_tag")
	}
}
