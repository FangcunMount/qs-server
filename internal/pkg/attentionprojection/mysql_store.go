package attentionprojection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type mysqlProjectionPO struct {
	EventID      string    `gorm:"column:event_id;primaryKey;size:128"`
	ReportID     string    `gorm:"column:report_id;size:64;not null"`
	AssessmentID string    `gorm:"column:assessment_id;size:64;not null"`
	TesteeID     uint64    `gorm:"column:testee_id;not null"`
	RiskLevel    string    `gorm:"column:risk_level;size:32;not null"`
	MarkKeyFocus bool      `gorm:"column:mark_key_focus;not null"`
	Status       Status    `gorm:"column:status;size:32;not null"`
	Attempt      int       `gorm:"column:attempt;not null"`
	LastError    string    `gorm:"column:last_error;type:text"`
	CreatedAt    time.Time `gorm:"column:created_at;not null"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null"`
}

func (mysqlProjectionPO) TableName() string { return "interpretation_attention_projection" }

// MySQLStore persists the asynchronous attention projection ledger. The
// immutable report fact source intentionally remains in MongoDB.
type MySQLStore struct {
	db *gorm.DB
}

func NewMySQLStore(db *gorm.DB) (*MySQLStore, error) {
	if db == nil {
		return nil, fmt.Errorf("mysql database is required")
	}
	return &MySQLStore{db: db}, nil
}

func (s *MySQLStore) EnsurePending(ctx context.Context, input PendingInput) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("attention projection store is not configured")
	}
	if input.EventID == "" {
		return false, fmt.Errorf("event_id is required")
	}
	now := time.Now().UTC()
	po := mysqlProjectionPO{
		EventID: input.EventID, ReportID: input.ReportID, AssessmentID: input.AssessmentID,
		TesteeID: input.TesteeID, RiskLevel: input.RiskLevel, MarkKeyFocus: input.MarkKeyFocus,
		Status: StatusPending, Attempt: 0, CreatedAt: now, UpdatedAt: now,
	}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "event_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"report_id", "assessment_id", "testee_id", "risk_level", "mark_key_focus", "updated_at",
		}),
	}).Create(&po).Error
	if err != nil {
		return false, fmt.Errorf("ensure attention projection pending: %w", err)
	}
	record, err := s.GetByEventID(ctx, input.EventID)
	if err != nil {
		return false, err
	}
	return record.Status == StatusSucceeded, nil
}

func (s *MySQLStore) MarkSucceeded(ctx context.Context, eventID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("attention projection store is not configured")
	}
	result := s.db.WithContext(ctx).Model(&mysqlProjectionPO{}).Where("event_id = ?", eventID).Updates(map[string]any{
		"status": StatusSucceeded, "last_error": "", "updated_at": time.Now().UTC(),
	})
	if result.Error != nil {
		return fmt.Errorf("mark attention projection succeeded: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: event=%s", ErrNotFound, eventID)
	}
	return nil
}

func (s *MySQLStore) RecordFailure(ctx context.Context, eventID string, errMsg string, maxAttempts int) (Status, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("attention projection store is not configured")
	}
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	var status Status
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var po mysqlProjectionPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("event_id = ?", eventID).First(&po).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: event=%s", ErrNotFound, eventID)
			}
			return err
		}
		attempt := po.Attempt + 1
		status = StatusFailed
		if attempt >= maxAttempts {
			status = StatusManualRequired
		}
		result := tx.Model(&mysqlProjectionPO{}).Where("event_id = ?", eventID).Updates(map[string]any{
			"attempt": attempt, "last_error": errMsg, "status": status, "updated_at": time.Now().UTC(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: event=%s", ErrNotFound, eventID)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("record attention projection failure: %w", err)
	}
	return status, nil
}

func (s *MySQLStore) GetByEventID(ctx context.Context, eventID string) (*Record, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("attention projection store is not configured")
	}
	var po mysqlProjectionPO
	if err := s.db.WithContext(ctx).Where("event_id = ?", eventID).First(&po).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: event=%s", ErrNotFound, eventID)
		}
		return nil, fmt.Errorf("find attention projection: %w", err)
	}
	return mysqlPOToRecord(&po), nil
}

func (s *MySQLStore) FindByReportID(ctx context.Context, reportID string) (*Record, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("attention projection store is not configured")
	}
	var po mysqlProjectionPO
	if err := s.db.WithContext(ctx).Where("report_id = ?", reportID).First(&po).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: report=%s", ErrNotFound, reportID)
		}
		return nil, fmt.Errorf("find attention projection by report: %w", err)
	}
	return mysqlPOToRecord(&po), nil
}

func (s *MySQLStore) ListRetryable(ctx context.Context, maxAttempts int, limit int) ([]Record, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("attention projection store is not configured")
	}
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	if limit <= 0 {
		limit = 100
	}
	var rows []mysqlProjectionPO
	if err := s.db.WithContext(ctx).
		Where("status IN ? AND attempt < ?", []Status{StatusPending, StatusFailed}, maxAttempts).
		Order("updated_at ASC, event_id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list retryable attention projections: %w", err)
	}
	items := make([]Record, 0, len(rows))
	for i := range rows {
		items = append(items, *mysqlPOToRecord(&rows[i]))
	}
	return items, nil
}

// ImportRecord inserts a historical Mongo ledger row without changing an
// already imported or newly written MySQL row. It is intended for the bounded
// cutover command, not runtime dual writes.
func (s *MySQLStore) ImportRecord(ctx context.Context, record Record) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("attention projection store is not configured")
	}
	if record.EventID == "" {
		return false, fmt.Errorf("event_id is required")
	}
	po := mysqlProjectionPO{
		EventID: record.EventID, ReportID: record.ReportID, AssessmentID: record.AssessmentID,
		TesteeID: record.TesteeID, RiskLevel: record.RiskLevel, MarkKeyFocus: record.MarkKeyFocus,
		Status: record.Status, Attempt: record.Attempt, LastError: record.LastError,
		CreatedAt: record.CreatedAt.UTC(), UpdatedAt: record.UpdatedAt.UTC(),
	}
	var imported bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current mysqlProjectionPO
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("event_id = ?", po.EventID).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&po).Error; err != nil {
				return err
			}
			imported = true
			return nil
		}
		if err != nil {
			return err
		}
		if !po.UpdatedAt.After(current.UpdatedAt) {
			return nil
		}
		result := tx.Model(&mysqlProjectionPO{}).Where("event_id = ?", po.EventID).Updates(map[string]any{
			"report_id": po.ReportID, "assessment_id": po.AssessmentID, "testee_id": po.TesteeID,
			"risk_level": po.RiskLevel, "mark_key_focus": po.MarkKeyFocus, "status": po.Status,
			"attempt": po.Attempt, "last_error": po.LastError, "updated_at": po.UpdatedAt,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: event=%s", ErrNotFound, po.EventID)
		}
		imported = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("import attention projection: %w", err)
	}
	return imported, nil
}

func mysqlPOToRecord(po *mysqlProjectionPO) *Record {
	if po == nil {
		return nil
	}
	return &Record{
		EventID: po.EventID, ReportID: po.ReportID, AssessmentID: po.AssessmentID,
		TesteeID: po.TesteeID, RiskLevel: po.RiskLevel, MarkKeyFocus: po.MarkKeyFocus,
		Status: po.Status, Attempt: po.Attempt, LastError: po.LastError,
		CreatedAt: po.CreatedAt, UpdatedAt: po.UpdatedAt,
	}
}

var _ Store = (*MySQLStore)(nil)
