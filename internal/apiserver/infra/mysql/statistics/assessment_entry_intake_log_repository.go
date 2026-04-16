package statistics

import (
	"context"
	"time"
)

// AssessmentEntryIntakeLogger 入口 intake 成功日志写入器。
type AssessmentEntryIntakeLogger struct {
	repo *StatisticsRepository
}

// NewAssessmentEntryIntakeLogger 创建入口 intake 日志写入器。
func NewAssessmentEntryIntakeLogger(repo *StatisticsRepository) *AssessmentEntryIntakeLogger {
	return &AssessmentEntryIntakeLogger{repo: repo}
}

// LogIntake 记录一次入口 intake 成功事件。
func (l *AssessmentEntryIntakeLogger) LogIntake(
	ctx context.Context,
	orgID int64,
	clinicianID, entryID, testeeID uint64,
	intakeAt time.Time,
	testeeCreated, assignmentCreated bool,
) error {
	po := &AssessmentEntryIntakeLogPO{
		OrgID:             orgID,
		ClinicianID:       clinicianID,
		EntryID:           entryID,
		TesteeID:          testeeID,
		TesteeCreated:     testeeCreated,
		AssignmentCreated: assignmentCreated,
		IntakeAt:          intakeAt,
	}
	return l.repo.WithContext(ctx).Create(po).Error
}
