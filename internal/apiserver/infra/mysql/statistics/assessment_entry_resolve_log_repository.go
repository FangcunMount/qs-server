package statistics

import (
	"context"
	"time"
)

// AssessmentEntryResolveLogger 入口解析日志写入器。
type AssessmentEntryResolveLogger struct {
	repo *StatisticsRepository
}

// NewAssessmentEntryResolveLogger 创建入口解析日志写入器。
func NewAssessmentEntryResolveLogger(repo *StatisticsRepository) *AssessmentEntryResolveLogger {
	return &AssessmentEntryResolveLogger{repo: repo}
}

// LogResolve 记录一次入口解析。
func (l *AssessmentEntryResolveLogger) LogResolve(ctx context.Context, orgID int64, clinicianID, entryID uint64, resolvedAt time.Time) error {
	po := &AssessmentEntryResolveLogPO{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		EntryID:     entryID,
		ResolvedAt:  resolvedAt,
	}
	return l.repo.WithContext(ctx).Create(po).Error
}
