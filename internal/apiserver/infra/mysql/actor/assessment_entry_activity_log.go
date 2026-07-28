package actor

import (
	"context"
	"time"

	gormuow "github.com/FangcunMount/component-base/pkg/uow/gorm"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"gorm.io/gorm"
)

// AssessmentEntryActivityLogRepository owns the immutable access activity
// sources consumed by Statistics. These rows are Actor business audit data,
// not Statistics projections.
type AssessmentEntryActivityLogRepository struct {
	db      *gorm.DB
	limiter backpressure.Acquirer
}

func NewAssessmentEntryActivityLogRepository(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) *AssessmentEntryActivityLogRepository {
	options := mysql.BaseRepositoryOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}
	return &AssessmentEntryActivityLogRepository{db: db, limiter: options.Limiter}
}

func (r *AssessmentEntryActivityLogRepository) create(ctx context.Context, value any) error {
	if r == nil || r.db == nil {
		return gorm.ErrInvalidDB
	}
	if r.limiter == nil {
		return gormuow.WithContext(ctx, r.db).Create(value).Error
	}
	limited, release, err := r.limiter.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return gormuow.WithContext(limited, r.db).Create(value).Error
}

type AssessmentEntryResolveLogger struct {
	repo *AssessmentEntryActivityLogRepository
}

func NewAssessmentEntryResolveLogger(repo *AssessmentEntryActivityLogRepository) *AssessmentEntryResolveLogger {
	return &AssessmentEntryResolveLogger{repo: repo}
}

func (l *AssessmentEntryResolveLogger) LogResolve(ctx context.Context, orgID int64, clinicianID, entryID uint64, resolvedAt time.Time) (uint64, error) {
	row := &assessmentEntryResolveLogPO{
		OrgID: orgID, ClinicianID: clinicianID, EntryID: entryID, ResolvedAt: resolvedAt,
	}
	if err := l.repo.create(ctx, row); err != nil {
		return 0, err
	}
	return row.ID, nil
}

type AssessmentEntryIntakeLogger struct {
	repo *AssessmentEntryActivityLogRepository
}

func NewAssessmentEntryIntakeLogger(repo *AssessmentEntryActivityLogRepository) *AssessmentEntryIntakeLogger {
	return &AssessmentEntryIntakeLogger{repo: repo}
}

func (l *AssessmentEntryIntakeLogger) LogIntake(ctx context.Context, orgID int64, clinicianID, entryID, testeeID uint64, intakeAt time.Time, testeeCreated, assignmentCreated bool) (uint64, error) {
	row := &assessmentEntryIntakeLogPO{
		OrgID: orgID, ClinicianID: clinicianID, EntryID: entryID, TesteeID: testeeID,
		IntakeAt: intakeAt, TesteeCreated: testeeCreated, AssignmentCreated: assignmentCreated,
	}
	if err := l.repo.create(ctx, row); err != nil {
		return 0, err
	}
	return row.ID, nil
}

type assessmentEntryResolveLogPO struct {
	ID, ClinicianID, EntryID uint64
	OrgID                    int64
	ResolvedAt               time.Time
	CreatedAt, UpdatedAt     time.Time
	DeletedAt                gorm.DeletedAt
}

func (assessmentEntryResolveLogPO) TableName() string { return "assessment_entry_resolve_log" }
func (p *assessmentEntryResolveLogPO) BeforeCreate(*gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

type assessmentEntryIntakeLogPO struct {
	ID, ClinicianID, EntryID, TesteeID uint64
	OrgID                              int64
	TesteeCreated, AssignmentCreated   bool
	IntakeAt                           time.Time
	CreatedAt, UpdatedAt               time.Time
	DeletedAt                          gorm.DeletedAt
}

func (assessmentEntryIntakeLogPO) TableName() string { return "assessment_entry_intake_log" }
func (p *assessmentEntryIntakeLogPO) BeforeCreate(*gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}
