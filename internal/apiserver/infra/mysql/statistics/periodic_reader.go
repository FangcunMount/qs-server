package statistics

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	actorInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	evaluationInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	planInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"gorm.io/gorm"
)

func (r *StatisticsRepository) GetPeriodicStats(ctx context.Context, orgID int64, testeeID uint64) (*domainStatistics.TesteePeriodicStatisticsResponse, error) {
	if err := r.ensureTesteeExists(ctx, orgID, testeeID); err != nil {
		return nil, err
	}
	tasks, err := r.loadPeriodicTasks(ctx, orgID, testeeID)
	if err != nil {
		return nil, err
	}
	projectMap, assessmentIDs := groupPeriodicTasksByPlan(tasks)
	assessmentNames, err := r.loadAssessmentNames(ctx, orgID, assessmentIDs)
	if err != nil {
		return nil, err
	}
	return buildPeriodicStatsResponse(projectMap, assessmentNames), nil
}

func (r *StatisticsRepository) ensureTesteeExists(ctx context.Context, orgID int64, testeeID uint64) error {
	var testee actorInfra.TesteePO
	if err := r.WithContext(ctx).
		Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, testeeID).
		First(&testee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return err
	}
	return nil
}

func (r *StatisticsRepository) loadPeriodicTasks(ctx context.Context, orgID int64, testeeID uint64) ([]planInfra.AssessmentTaskPO, error) {
	var tasks []planInfra.AssessmentTaskPO
	if err := r.WithContext(ctx).
		Table("assessment_task t").
		Joins("JOIN assessment_plan p ON p.id = t.plan_id AND p.deleted_at IS NULL").
		Where("t.org_id = ? AND t.testee_id = ? AND t.deleted_at IS NULL", orgID, testeeID).
		Order("t.plan_id ASC, t.seq ASC, t.planned_at ASC, t.id ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func groupPeriodicTasksByPlan(tasks []planInfra.AssessmentTaskPO) (map[string][]planInfra.AssessmentTaskPO, []uint64) {
	projectMap := make(map[string][]planInfra.AssessmentTaskPO)
	assessmentIDs := make([]uint64, 0)
	for _, item := range tasks {
		planID := strconv.FormatUint(item.PlanID, 10)
		projectMap[planID] = append(projectMap[planID], item)
		if item.AssessmentID != nil {
			assessmentIDs = append(assessmentIDs, *item.AssessmentID)
		}
	}
	return projectMap, assessmentIDs
}

func (r *StatisticsRepository) loadAssessmentNames(ctx context.Context, orgID int64, assessmentIDs []uint64) (map[uint64]string, error) {
	assessmentNames := make(map[uint64]string, len(assessmentIDs))
	if len(assessmentIDs) == 0 {
		return assessmentNames, nil
	}
	var assessments []evaluationInfra.AssessmentPO
	if err := r.WithContext(ctx).
		Select("id, medical_scale_name").
		Where("org_id = ? AND id IN ? AND deleted_at IS NULL", orgID, assessmentIDs).
		Find(&assessments).Error; err != nil {
		return nil, err
	}
	for _, item := range assessments {
		if item.MedicalScaleName != nil && strings.TrimSpace(*item.MedicalScaleName) != "" {
			assessmentNames[item.ID.Uint64()] = strings.TrimSpace(*item.MedicalScaleName)
		}
	}
	return assessmentNames, nil
}

func buildPeriodicStatsResponse(projectMap map[string][]planInfra.AssessmentTaskPO, assessmentNames map[uint64]string) *domainStatistics.TesteePeriodicStatisticsResponse {
	projects := make([]domainStatistics.TesteePeriodicProjectStatistics, 0, len(projectMap))
	activeProjects := 0
	for planID, items := range projectMap {
		project, hasActiveTask := buildPeriodicProjectStatistics(planID, items, assessmentNames)
		if hasActiveTask {
			activeProjects++
		}
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ProjectID < projects[j].ProjectID
	})
	return &domainStatistics.TesteePeriodicStatisticsResponse{
		Projects:       projects,
		TotalProjects:  len(projects),
		ActiveProjects: activeProjects,
	}
}

func buildPeriodicProjectStatistics(planID string, items []planInfra.AssessmentTaskPO, assessmentNames map[uint64]string) (domainStatistics.TesteePeriodicProjectStatistics, bool) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Seq == items[j].Seq {
			if items[i].PlannedAt.Equal(items[j].PlannedAt) {
				return items[i].ID < items[j].ID
			}
			return items[i].PlannedAt.Before(items[j].PlannedAt)
		}
		return items[i].Seq < items[j].Seq
	})

	completed := 0
	currentWeek := 0
	hasActiveTask := false
	scaleName := ""
	tasks := make([]domainStatistics.TesteePeriodicTaskStatistics, 0, len(items))
	var startDate *time.Time
	var endDate *time.Time

	for _, item := range items {
		task := buildPeriodicTaskStatistics(item)
		tasks = append(tasks, task)
		if task.Status == "completed" {
			completed++
		} else if currentWeek == 0 {
			currentWeek = item.Seq
		}
		if item.Status == "pending" || item.Status == "opened" {
			hasActiveTask = true
		}
		scaleName = pickPeriodicScaleName(scaleName, item, assessmentNames)
		startDate, endDate = expandPeriodicWindow(startDate, endDate, item)
	}

	totalWeeks := len(items)
	if totalWeeks == 0 {
		currentWeek = 0
	} else if currentWeek == 0 {
		currentWeek = totalWeeks
	}
	if scaleName == "" {
		scaleName = "未命名量表"
	}

	return domainStatistics.TesteePeriodicProjectStatistics{
		ProjectID:      planID,
		ProjectName:    scaleName,
		ScaleName:      scaleName,
		TotalWeeks:     totalWeeks,
		CompletedWeeks: completed,
		CompletionRate: calculatePeriodicCompletionRate(completed, totalWeeks),
		CurrentWeek:    currentWeek,
		Tasks:          tasks,
		StartDate:      startDate,
		EndDate:        endDate,
	}, hasActiveTask
}

func buildPeriodicTaskStatistics(item planInfra.AssessmentTaskPO) domainStatistics.TesteePeriodicTaskStatistics {
	task := domainStatistics.TesteePeriodicTaskStatistics{
		Week:      item.Seq,
		Status:    periodicTaskStatus(item.Status),
		PlannedAt: cloneTime(item.PlannedAt),
		DueDate:   cloneTimePtr(item.ExpireAt),
	}
	if item.CompletedAt != nil {
		task.CompletedAt = cloneTimePtr(item.CompletedAt)
	}
	if item.AssessmentID != nil {
		assessmentID := strconv.FormatUint(*item.AssessmentID, 10)
		task.AssessmentID = &assessmentID
	}
	return task
}

func pickPeriodicScaleName(current string, item planInfra.AssessmentTaskPO, assessmentNames map[uint64]string) string {
	if current != "" {
		return current
	}
	if item.AssessmentID != nil && assessmentNames[*item.AssessmentID] != "" {
		return assessmentNames[*item.AssessmentID]
	}
	return strings.TrimSpace(item.ScaleCode)
}

func expandPeriodicWindow(startDate, endDate *time.Time, item planInfra.AssessmentTaskPO) (*time.Time, *time.Time) {
	if startDate == nil || item.PlannedAt.Before(*startDate) {
		startDate = cloneTime(item.PlannedAt)
	}
	if item.ExpireAt != nil {
		if endDate == nil || item.ExpireAt.After(*endDate) {
			endDate = cloneTimePtr(item.ExpireAt)
		}
		return startDate, endDate
	}
	if endDate == nil || item.PlannedAt.After(*endDate) {
		endDate = cloneTime(item.PlannedAt)
	}
	return startDate, endDate
}

func calculatePeriodicCompletionRate(completed, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(completed) / float64(total) * 100
}

func periodicTaskStatus(status string) string {
	switch status {
	case "completed":
		return "completed"
	case "expired":
		return "overdue"
	case "canceled":
		return "canceled"
	default:
		return "pending"
	}
}

func cloneTime(value time.Time) *time.Time {
	t := value
	return &t
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	t := *value
	return &t
}
