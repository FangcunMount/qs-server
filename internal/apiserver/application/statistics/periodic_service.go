package statistics

import (
	"context"
	"fmt"
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

type periodicStatsService struct {
	db *gorm.DB
}

// NewPeriodicStatsService 创建受试者周期统计服务。
func NewPeriodicStatsService(db *gorm.DB) PeriodicStatsService {
	return &periodicStatsService{db: db}
}

func (s *periodicStatsService) GetPeriodicStats(ctx context.Context, orgID int64, testeeID uint64) (*domainStatistics.TesteePeriodicStatisticsResponse, error) {
	var testee actorInfra.TesteePO
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, testeeID).
		First(&testee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, err
	}

	var tasks []planInfra.AssessmentTaskPO
	if err := s.db.WithContext(ctx).
		Table("assessment_task t").
		Joins("JOIN assessment_plan p ON p.id = t.plan_id AND p.deleted_at IS NULL").
		Where("t.org_id = ? AND t.testee_id = ? AND t.deleted_at IS NULL", orgID, testeeID).
		Order("t.plan_id ASC, t.seq ASC, t.planned_at ASC, t.id ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}

	projectMap := make(map[string][]planInfra.AssessmentTaskPO)
	assessmentIDs := make([]uint64, 0)
	for _, item := range tasks {
		planID := strconv.FormatUint(item.PlanID, 10)
		projectMap[planID] = append(projectMap[planID], item)
		if item.AssessmentID != nil {
			assessmentIDs = append(assessmentIDs, *item.AssessmentID)
		}
	}

	assessmentNames := make(map[uint64]string, len(assessmentIDs))
	if len(assessmentIDs) > 0 {
		var assessments []evaluationInfra.AssessmentPO
		if err := s.db.WithContext(ctx).
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
	}

	projects := make([]domainStatistics.TesteePeriodicProjectStatistics, 0, len(projectMap))
	activeProjects := 0
	for planID, items := range projectMap {
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
		tasks := make([]domainStatistics.TesteePeriodicTaskStatistics, 0, len(items))
		var startDate *time.Time
		var endDate *time.Time
		currentWeek := 0
		hasActiveTask := false
		scaleName := ""
		for _, item := range items {
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

			if task.Status == "completed" {
				completed++
			} else if currentWeek == 0 {
				currentWeek = item.Seq
			}
			if item.Status == "pending" || item.Status == "opened" {
				hasActiveTask = true
			}
			if scaleName == "" {
				if item.AssessmentID != nil {
					scaleName = assessmentNames[*item.AssessmentID]
				}
				if scaleName == "" {
					scaleName = strings.TrimSpace(item.ScaleCode)
				}
			}
			if startDate == nil || item.PlannedAt.Before(*startDate) {
				startDate = cloneTime(item.PlannedAt)
			}
			if item.ExpireAt != nil {
				if endDate == nil || item.ExpireAt.After(*endDate) {
					endDate = cloneTimePtr(item.ExpireAt)
				}
			} else if endDate == nil || item.PlannedAt.After(*endDate) {
				endDate = cloneTime(item.PlannedAt)
			}
			tasks = append(tasks, task)
		}

		totalWeeks := len(items)
		completionRate := 0.0
		if totalWeeks > 0 {
			completionRate = float64(completed) / float64(totalWeeks) * 100
		}
		if totalWeeks == 0 {
			currentWeek = 0
		} else if currentWeek == 0 {
			currentWeek = totalWeeks
		}
		if hasActiveTask {
			activeProjects++
		}
		if scaleName == "" {
			scaleName = "未命名量表"
		}

		projects = append(projects, domainStatistics.TesteePeriodicProjectStatistics{
			ProjectID:      planID,
			ProjectName:    fmt.Sprintf("计划 %s", planID),
			ScaleName:      scaleName,
			TotalWeeks:     totalWeeks,
			CompletedWeeks: completed,
			CompletionRate: completionRate,
			CurrentWeek:    currentWeek,
			Tasks:          tasks,
			StartDate:      startDate,
			EndDate:        endDate,
		})
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ProjectID < projects[j].ProjectID
	})

	return &domainStatistics.TesteePeriodicStatisticsResponse{
		Projects:       projects,
		TotalProjects:  len(projects),
		ActiveProjects: activeProjects,
	}, nil
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
