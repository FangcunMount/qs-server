package plan

import (
	"context"
	"strconv"
	"strings"
	"time"

	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"gorm.io/gorm"
)

type EnrollmentReadStore struct {
	db      *gorm.DB
	limiter backpressure.Acquirer
}

func NewEnrollmentReadStore(db *gorm.DB, limiter backpressure.Acquirer) *EnrollmentReadStore {
	return &EnrollmentReadStore{db: db, limiter: limiter}
}

func (s *EnrollmentReadStore) ListEnrollments(ctx context.Context, query planapp.EnrollmentQuery) ([]planapp.EnrollmentItem, int64, error) {
	if s.limiter != nil {
		var release func()
		var err error
		ctx, release, err = s.limiter.Acquire(ctx)
		if err != nil {
			return nil, 0, err
		}
		defer release()
	}
	where := []string{"org_id=?", "testee_id=?", "deleted_at IS NULL"}
	args := []any{query.OrgID, query.TesteeID}
	if query.PlanID != nil {
		where = append(where, "plan_id=?")
		args = append(args, *query.PlanID)
	}
	if query.Status != "" {
		where = append(where, "status=?")
		args = append(args, query.Status)
	}
	handle := s.db.WithContext(ctx).Table("plan_enrollment").Where(strings.Join(where, " AND "), args...)
	var total int64
	if err := handle.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []planapp.EnrollmentItem
	if err := handle.Order("joined_at DESC,id DESC").Limit(query.PageSize).Offset((query.Page - 1) * query.PageSize).Scan(&items).Error; err != nil {
		return nil, 0, err
	}
	if len(items) == 0 {
		return items, total, nil
	}
	ids := make([]uint64, 0, len(items))
	index := make(map[uint64]int, len(items))
	for i := range items {
		ids = append(ids, items[i].ID)
		index[items[i].ID] = i
		items[i].Tasks = []planapp.EnrollmentTaskItem{}
	}
	type taskRow struct {
		ID, EnrollmentID                                     uint64
		Seq                                                  int
		ScaleCode, Status                                    string
		PlannedAt                                            time.Time
		OpenAt, ExpireAt, CompletedAt, ExpiredAt, CanceledAt *time.Time
		AssessmentID                                         *uint64
	}
	var tasks []taskRow
	if err := s.db.WithContext(ctx).Table("assessment_task").Select("id,enrollment_id,seq,scale_code,status,planned_at,open_at,expire_at,completed_at,expired_at,canceled_at,assessment_id").Where("enrollment_id IN ? AND deleted_at IS NULL", ids).Order("enrollment_id,seq").Scan(&tasks).Error; err != nil {
		return nil, 0, err
	}
	for _, task := range tasks {
		if position, ok := index[task.EnrollmentID]; ok {
			value := planapp.EnrollmentTaskItem{ID: task.ID, Seq: task.Seq, ScaleCode: task.ScaleCode, Status: task.Status, PlannedAt: task.PlannedAt, OpenAt: task.OpenAt, ExpireAt: task.ExpireAt, CompletedAt: task.CompletedAt, ExpiredAt: task.ExpiredAt, CanceledAt: task.CanceledAt}
			if task.AssessmentID != nil {
				text := strconv.FormatUint(*task.AssessmentID, 10)
				value.AssessmentID = &text
			}
			items[position].Tasks = append(items[position].Tasks, value)
		}
	}
	return items, total, nil
}
