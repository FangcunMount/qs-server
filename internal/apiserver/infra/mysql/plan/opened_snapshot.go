package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"gorm.io/gorm"
)

// SaveOpenedSnapshot only updates a previously persisted pending schedule in
// its original caller transaction. IDs, entry, clocks and actor were frozen
// before half preparation. It does not create missing tasks or emit events.
func (r *taskRepository) SaveOpenedSnapshot(ctx context.Context, po *AssessmentTaskPO, at time.Time, actor uint64) error {
	if _, err := mysql.RequireTx(ctx); err != nil {
		return err
	}
	if po == nil || po.ID.IsZero() || po.Status != domain.TaskStatusOpened.String() || po.OpenAt == nil || at.IsZero() || !at.Equal(*po.OpenAt) || po.EntryURL == "" {
		return transaction.ErrInvalidPlan
	}
	updates := map[string]any{"status": po.Status, "open_at": po.OpenAt, "expire_at": po.ExpireAt, "entry_url": po.EntryURL, "updated_at": at, "version": gorm.Expr("version + 1")}
	if actor > 0 {
		updates["updated_by"] = actor
	}
	result := r.WithContext(ctx).Model(&AssessmentTaskPO{}).
		Where("id = ? AND org_id = ? AND plan_id = ? AND enrollment_id = ? AND testee_id = ? AND seq = ? AND scale_code = ? AND status = ? AND schedule_revision = ? AND open_at IS NULL AND deleted_at IS NULL",
			po.ID.Uint64(), po.OrgID, po.PlanID, po.EnrollmentID, po.TesteeID, po.Seq, po.ScaleCode, domain.TaskStatusPending.String(), po.ScheduleRevision).
		Updates(updates)
	if result.Error != nil {
		return translateTaskError(result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.WithCode(code.ErrConflict, "task opening lost original pending schedule")
	}
	return nil
}
