package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
)

type enrollmentRepository struct {
	mysql.BaseRepository[*PlanEnrollmentPO]
}

func (r *enrollmentRepository) CloseIfAllTasksTerminal(ctx context.Context, id domainplan.PlanEnrollmentID, closedAt time.Time) (bool, error) {
	result := r.WithContext(ctx).Exec(`
		UPDATE plan_enrollment e
		SET e.status = ?, e.closed_at = ?, e.updated_at = ?
		WHERE e.id = ? AND e.status = ? AND e.deleted_at IS NULL
		  AND NOT EXISTS (
			SELECT 1 FROM assessment_task t
			WHERE t.enrollment_id = e.id AND t.deleted_at IS NULL
			  AND t.status NOT IN (?, ?, ?)
		  )`,
		domainplan.EnrollmentStatusClosed, closedAt, closedAt, id.Uint64(), domainplan.EnrollmentStatusActive,
		domainplan.TaskStatusCompleted, domainplan.TaskStatusExpired, domainplan.TaskStatusCanceled,
	)
	return result.RowsAffected > 0, result.Error
}

func (r *enrollmentRepository) TerminateActiveByPlan(ctx context.Context, orgID int64, planID domainplan.AssessmentPlanID, reason string, at time.Time) (int64, error) {
	result := r.WithContext(ctx).Model(&PlanEnrollmentPO{}).Where("org_id=? AND plan_id=? AND status=? AND deleted_at IS NULL", orgID, planID.Uint64(), domainplan.EnrollmentStatusActive).Updates(map[string]any{"status": domainplan.EnrollmentStatusTerminated, "terminated_at": at, "terminated_reason": reason, "updated_at": at})
	return result.RowsAffected, result.Error
}

func (r *enrollmentRepository) CloseActiveByPlanIfAllTasksTerminal(ctx context.Context, orgID int64, planID domainplan.AssessmentPlanID, at time.Time) (int64, error) {
	result := r.WithContext(ctx).Exec(`UPDATE plan_enrollment e SET e.status=?,e.closed_at=?,e.updated_at=?
		WHERE e.org_id=? AND e.plan_id=? AND e.status=? AND e.deleted_at IS NULL
		AND NOT EXISTS (SELECT 1 FROM assessment_task t WHERE t.enrollment_id=e.id AND t.deleted_at IS NULL AND t.status NOT IN ('completed','expired','canceled'))`,
		domainplan.EnrollmentStatusClosed, at, at, orgID, planID.Uint64(), domainplan.EnrollmentStatusActive)
	return result.RowsAffected, result.Error
}

func NewEnrollmentRepository(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) domainplan.EnrollmentRepository {
	return &enrollmentRepository{BaseRepository: mysql.NewBaseRepository[*PlanEnrollmentPO](db, opts...)}
}

func (r *enrollmentRepository) FindByID(ctx context.Context, id domainplan.PlanEnrollmentID) (*domainplan.Enrollment, error) {
	po, err := r.BaseRepository.FindByID(ctx, id.Uint64())
	if err != nil {
		return nil, err
	}
	return enrollmentToDomain(po), nil
}

func (r *enrollmentRepository) FindActive(ctx context.Context, orgID int64, planID domainplan.AssessmentPlanID, testeeID testee.ID) (*domainplan.Enrollment, error) {
	var po PlanEnrollmentPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND plan_id = ? AND testee_id = ? AND status = ? AND deleted_at IS NULL", orgID, planID.Uint64(), testeeID.Uint64(), domainplan.EnrollmentStatusActive).
		Take(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return enrollmentToDomain(&po), nil
}

func (r *enrollmentRepository) FindLatest(ctx context.Context, orgID int64, planID domainplan.AssessmentPlanID, testeeID testee.ID) (*domainplan.Enrollment, error) {
	var po PlanEnrollmentPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND plan_id = ? AND testee_id = ? AND deleted_at IS NULL", orgID, planID.Uint64(), testeeID.Uint64()).
		Order("round DESC").Take(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return enrollmentToDomain(&po), nil
}

func (r *enrollmentRepository) Save(ctx context.Context, enrollment *domainplan.Enrollment) error {
	po := enrollmentToPO(enrollment)
	exists, err := r.ExistsByID(ctx, po.ID.Uint64())
	if err != nil {
		return err
	}
	if !exists {
		if err := r.CreateAndSync(ctx, po, nil); err != nil {
			if mysql.IsDuplicateError(err) {
				return domainplan.ErrActiveEnrollmentExists
			}
			return err
		}
		return nil
	}
	return r.UpdateAndSync(ctx, po, nil)
}

func enrollmentToPO(enrollment *domainplan.Enrollment) *PlanEnrollmentPO {
	return &PlanEnrollmentPO{
		AuditFields: mysql.AuditFields{ID: enrollment.ID()},
		OrgID:       enrollment.OrgID(), PlanID: enrollment.PlanID().Uint64(), TesteeID: enrollment.TesteeID().Uint64(),
		Round: enrollment.Round(), StartDate: enrollment.StartDate(), Status: string(enrollment.Status()),
		JoinedAt: enrollment.JoinedAt(), ClosedAt: enrollment.ClosedAt(), TerminatedAt: enrollment.TerminatedAt(),
		TerminatedReason: enrollment.TerminatedReason(), RecordOrigin: string(enrollment.RecordOrigin()),
	}
}

func enrollmentToDomain(po *PlanEnrollmentPO) *domainplan.Enrollment {
	return domainplan.RestoreEnrollment(
		po.ID, po.OrgID, meta.FromUint64(po.PlanID), testee.ID(meta.FromUint64(po.TesteeID)),
		po.Round, po.StartDate, domainplan.EnrollmentStatus(po.Status), po.JoinedAt, po.ClosedAt, po.TerminatedAt,
		po.TerminatedReason, domainplan.EnrollmentRecordOrigin(po.RecordOrigin),
	)
}
