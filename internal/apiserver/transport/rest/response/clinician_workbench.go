package response

import (
	"fmt"

	workbenchApp "github.com/FangcunMount/qs-server/internal/apiserver/application/workbench"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
)

type ClinicianWorkbenchQueueSummaryResponse struct {
	Counts ClinicianWorkbenchQueueCountsResponse `json:"counts"`
}

type ClinicianWorkbenchQueueCountsResponse struct {
	HighRisk int64 `json:"high_risk"`
	FollowUp int64 `json:"follow_up"`
	KeyFocus int64 `json:"key_focus"`
}

type ClinicianWorkbenchQueueResponse struct {
	QueueType  string                                `json:"queue_type"`
	Items      []ClinicianWorkbenchQueueItemResponse `json:"items"`
	Total      int64                                 `json:"total"`
	Page       int                                   `json:"page"`
	PageSize   int                                   `json:"page_size"`
	TotalPages int                                   `json:"total_pages"`
}

type ClinicianWorkbenchQueueItemResponse struct {
	Testee             *TesteeResponse                        `json:"testee"`
	ReasonCode         string                                 `json:"reason_code"`
	Reason             string                                 `json:"reason"`
	ReasonAt           *string                                `json:"reason_at,omitempty"`
	RiskLevel          string                                 `json:"risk_level,omitempty"`
	Task               *ClinicianWorkbenchTaskSummaryResponse `json:"task"`
	PrimaryClinician   *ClinicianAssignmentResponse           `json:"primary_clinician,omitempty"`
	AssignedClinicians []ClinicianAssignmentResponse          `json:"assigned_clinicians,omitempty"`
	IsUnassigned       *bool                                  `json:"is_unassigned,omitempty"`
}

type ClinicianWorkbenchTaskSummaryResponse struct {
	TaskID      string  `json:"task_id"`
	PlanID      string  `json:"plan_id"`
	Status      string  `json:"status"`
	StatusLabel string  `json:"status_label,omitempty"`
	PlannedAt   string  `json:"planned_at"`
	OpenAt      *string `json:"open_at,omitempty"`
	ExpireAt    *string `json:"expire_at,omitempty"`
	ScaleCode   string  `json:"scale_code"`
	EntryURL    string  `json:"entry_url,omitempty"`
}

type ClinicianAssignmentResponse struct {
	ID            string  `json:"id"`
	OrgID         string  `json:"org_id"`
	OperatorID    *string `json:"operator_id,omitempty"`
	Name          string  `json:"name"`
	Department    string  `json:"department,omitempty"`
	Title         string  `json:"title,omitempty"`
	ClinicianType string  `json:"clinician_type"`
	RelationType  string  `json:"relation_type"`
	BoundAt       string  `json:"bound_at"`
}

func NewClinicianWorkbenchQueueSummaryResponse(result *workbenchApp.SummaryResult) *ClinicianWorkbenchQueueSummaryResponse {
	if result == nil {
		return &ClinicianWorkbenchQueueSummaryResponse{}
	}
	return &ClinicianWorkbenchQueueSummaryResponse{
		Counts: ClinicianWorkbenchQueueCountsResponse{
			HighRisk: result.Counts.HighRisk,
			FollowUp: result.Counts.FollowUp,
			KeyFocus: result.Counts.KeyFocus,
		},
	}
}

func NewClinicianWorkbenchQueueResponse(result *workbenchApp.QueuePage) *ClinicianWorkbenchQueueResponse {
	if result == nil {
		return &ClinicianWorkbenchQueueResponse{Items: []ClinicianWorkbenchQueueItemResponse{}}
	}
	items := make([]ClinicianWorkbenchQueueItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, newClinicianWorkbenchQueueItemResponse(item))
	}
	return &ClinicianWorkbenchQueueResponse{
		QueueType:  string(result.QueueType),
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}
}

func newClinicianWorkbenchQueueItemResponse(item workbenchApp.QueueItem) ClinicianWorkbenchQueueItemResponse {
	return ClinicianWorkbenchQueueItemResponse{
		Testee:             newClinicianWorkbenchTesteeResponse(item.Testee),
		ReasonCode:         item.ReasonCode,
		Reason:             item.Reason,
		ReasonAt:           FormatDateTimePtr(item.ReasonAt),
		RiskLevel:          item.RiskLevel,
		Task:               newClinicianWorkbenchTaskSummaryResponse(item.Task),
		PrimaryClinician:   newClinicianAssignmentResponse(item.PrimaryClinician),
		AssignedClinicians: newClinicianAssignmentResponses(item.AssignedClinicians),
		IsUnassigned:       item.IsUnassigned,
	}
}

func newClinicianWorkbenchTesteeResponse(result workbenchApp.Testee) *TesteeResponse {
	gender := GenderCodeFromValue(result.Gender)
	idStr := fmt.Sprintf("%d", result.ID)
	orgIDStr := fmt.Sprintf("%d", result.OrgID)
	var profileIDStr *string
	if result.ProfileID != nil {
		s := fmt.Sprintf("%d", *result.ProfileID)
		profileIDStr = &s
	}
	resp := &TesteeResponse{
		ID:              idStr,
		OrgID:           orgIDStr,
		ProfileID:       profileIDStr,
		IAMChildID:      LegacyIAMChildIDAlias(profileIDStr),
		Name:            result.Name,
		Gender:          gender,
		GenderLabel:     LabelForGender(gender),
		Birthday:        FormatDatePtr(result.Birthday),
		Tags:            append([]string(nil), result.Tags...),
		TagsLabel:       LabelTags(result.Tags),
		Source:          result.Source,
		SourceLabel:     LabelForTesteeSource(result.Source),
		IsKeyFocus:      result.IsKeyFocus,
		IsKeyFocusLabel: LabelForKeyFocus(result.IsKeyFocus),
		CreatedAt:       FormatDateTimeValue(result.CreatedAt),
		UpdatedAt:       FormatDateTimeValue(result.UpdatedAt),
	}
	if result.LastAssessmentAt != nil || result.TotalAssessments > 0 || result.LastRiskLevel != "" {
		resp.AssessmentStats = &AssessmentStatsResponse{
			TotalCount:         result.TotalAssessments,
			LastAssessmentAt:   FormatDateTimePtr(result.LastAssessmentAt),
			LastRiskLevel:      result.LastRiskLevel,
			LastRiskLevelLabel: LabelForRiskLevel(result.LastRiskLevel),
		}
	}
	return resp
}

func newClinicianWorkbenchTaskSummaryResponse(task *workbenchApp.TaskSummary) *ClinicianWorkbenchTaskSummaryResponse {
	if task == nil {
		return nil
	}
	return &ClinicianWorkbenchTaskSummaryResponse{
		TaskID:      fmt.Sprintf("%d", task.TaskID),
		PlanID:      fmt.Sprintf("%d", task.PlanID),
		Status:      task.Status,
		StatusLabel: domainPlan.TaskStatus(task.Status).DisplayName(),
		PlannedAt:   FormatDateTimeValue(task.PlannedAt),
		OpenAt:      FormatDateTimePtr(task.OpenAt),
		ExpireAt:    FormatDateTimePtr(task.ExpireAt),
		ScaleCode:   task.ScaleCode,
		EntryURL:    task.EntryURL,
	}
}

func newClinicianAssignmentResponse(item *workbenchApp.ClinicianAssignment) *ClinicianAssignmentResponse {
	if item == nil {
		return nil
	}
	var operatorID *string
	if item.OperatorID != nil {
		value := fmt.Sprintf("%d", *item.OperatorID)
		operatorID = &value
	}
	return &ClinicianAssignmentResponse{
		ID:            fmt.Sprintf("%d", item.ID),
		OrgID:         fmt.Sprintf("%d", item.OrgID),
		OperatorID:    operatorID,
		Name:          item.Name,
		Department:    item.Department,
		Title:         item.Title,
		ClinicianType: item.ClinicianType,
		RelationType:  item.RelationType,
		BoundAt:       FormatDateTimeValue(item.BoundAt),
	}
}

func newClinicianAssignmentResponses(items []workbenchApp.ClinicianAssignment) []ClinicianAssignmentResponse {
	if len(items) == 0 {
		return nil
	}
	result := make([]ClinicianAssignmentResponse, 0, len(items))
	for i := range items {
		item := newClinicianAssignmentResponse(&items[i])
		if item != nil {
			result = append(result, *item)
		}
	}
	return result
}
