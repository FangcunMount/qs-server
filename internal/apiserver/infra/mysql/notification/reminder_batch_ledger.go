package notification

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	appnotification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	"gorm.io/gorm"
)

type reminderBatchPO struct {
	ID               uint64    `gorm:"column:id;primaryKey"`
	OrgID            int64     `gorm:"column:org_id"`
	TaskID           string    `gorm:"column:task_id"`
	OpeningEventID   string    `gorm:"column:opening_event_id"`
	ScheduleRevision uint32    `gorm:"column:schedule_revision"`
	ReminderVersion  uint32    `gorm:"column:reminder_version"`
	AppID            string    `gorm:"column:app_id"`
	TemplateID       string    `gorm:"column:template_id"`
	RecipientCount   uint32    `gorm:"column:recipient_count"`
	State            string    `gorm:"column:state"`
	ResolutionCode   string    `gorm:"column:resolution_code"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (reminderBatchPO) TableName() string { return "task_opened_reminder_batch" }

type ReminderBatchLedger struct{ db *gorm.DB }

func NewReminderBatchLedger(db *gorm.DB) *ReminderBatchLedger {
	return &ReminderBatchLedger{db: db}
}

func (s *ReminderBatchLedger) FreezeRecipients(
	ctx context.Context, key appnotification.ReminderBatchKey, appID, templateID string,
	recipients []appnotification.ReminderRecipientIdentity, now time.Time,
) (appnotification.ReminderBatch, error) {
	if err := s.validateBatch(key, appID, templateID, now); err != nil {
		return appnotification.ReminderBatch{}, err
	}
	if len(recipients) == 0 {
		return appnotification.ReminderBatch{}, fmt.Errorf("freeze requires a nonempty recipient set")
	}
	seen := make(map[string]struct{}, len(recipients))
	for _, recipient := range recipients {
		if err := validateIdentity(recipient.UserID, 64, "user ID"); err != nil {
			return appnotification.ReminderBatch{}, err
		}
		if err := validateIdentity(recipient.LoginIdentityID, 64, "login identity ID"); err != nil {
			return appnotification.ReminderBatch{}, err
		}
		if _, exists := seen[recipient.LoginIdentityID]; exists {
			return appnotification.ReminderBatch{}, fmt.Errorf("duplicate login identity in recipient set")
		}
		seen[recipient.LoginIdentityID] = struct{}{}
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inserted, err := insertReminderBatch(tx, key, appID, templateID, "frozen", "", len(recipients), now)
		if err != nil || !inserted {
			return err
		}
		for _, recipient := range recipients {
			if err := tx.Exec(`INSERT INTO task_opened_reminder_delivery
				(org_id,task_id,opening_event_id,schedule_revision,user_id,login_identity_id,
				 app_id,template_id,reminder_version,state,created_at,updated_at)
				VALUES (?,?,?,?,?,?,?,?,?,'pending',?,?)`,
				key.OrgID, key.TaskID, key.OpeningEventID, key.ScheduleRevision,
				recipient.UserID, recipient.LoginIdentityID, appID, templateID,
				key.ReminderVersion, now, now,
			).Error; err != nil {
				return fmt.Errorf("freeze reminder recipient: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return appnotification.ReminderBatch{}, fmt.Errorf("freeze reminder batch: %w", err)
	}
	return s.ReadBatch(ctx, key)
}

// SuppressEmpty records a final no-send outcome for an opening with no
// recipient responsibility. It must only be called after the caller has
// determined that the opening cannot be sent within its product window.
func (s *ReminderBatchLedger) SuppressEmpty(
	ctx context.Context, key appnotification.ReminderBatchKey, appID, templateID, code string, now time.Time,
) (appnotification.ReminderBatch, error) {
	if err := s.validateBatch(key, appID, templateID, now); err != nil {
		return appnotification.ReminderBatch{}, err
	}
	if err := validateResolutionCode(code); err != nil {
		return appnotification.ReminderBatch{}, err
	}
	if _, err := insertReminderBatch(s.db.WithContext(ctx), key, appID, templateID, "suppressed", code, 0, now); err != nil {
		return appnotification.ReminderBatch{}, err
	}
	return s.ReadBatch(ctx, key)
}

func insertReminderBatch(
	db *gorm.DB, key appnotification.ReminderBatchKey, appID, templateID, state, code string, recipientCount int, now time.Time,
) (bool, error) {
	result := db.Exec(`INSERT IGNORE INTO task_opened_reminder_batch
		(org_id,task_id,opening_event_id,schedule_revision,reminder_version,
		 app_id,template_id,recipient_count,state,resolution_code,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		key.OrgID, key.TaskID, key.OpeningEventID, key.ScheduleRevision, key.ReminderVersion,
		appID, templateID, recipientCount, state, code, now, now,
	)
	if result.Error != nil {
		return false, fmt.Errorf("create reminder batch: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (s *ReminderBatchLedger) ReadBatch(
	ctx context.Context, key appnotification.ReminderBatchKey,
) (appnotification.ReminderBatch, error) {
	if err := s.validateBatchKey(key); err != nil {
		return appnotification.ReminderBatch{}, err
	}
	var row reminderBatchPO
	if err := s.batchQuery(ctx, key).First(&row).Error; err != nil {
		return appnotification.ReminderBatch{}, fmt.Errorf("read reminder batch: %w", err)
	}
	batch := appnotification.ReminderBatch{
		Key: key, AppID: row.AppID, TemplateID: row.TemplateID,
		Suppressed: row.State == "suppressed", ResolutionCode: row.ResolutionCode,
	}
	if row.State != "frozen" && row.State != "suppressed" {
		return appnotification.ReminderBatch{}, fmt.Errorf("unknown reminder batch state")
	}
	var recipients []reminderDeliveryPO
	if err := s.db.WithContext(ctx).Where(
		"org_id = ? AND task_id = ? AND opening_event_id = ? AND schedule_revision = ? AND reminder_version = ?",
		key.OrgID, key.TaskID, key.OpeningEventID, key.ScheduleRevision, key.ReminderVersion,
	).Find(&recipients).Error; err != nil {
		return appnotification.ReminderBatch{}, fmt.Errorf("read reminder batch recipients: %w", err)
	}
	if len(recipients) != int(row.RecipientCount) || row.State == "frozen" && len(recipients) == 0 || row.State == "suppressed" && len(recipients) != 0 {
		return appnotification.ReminderBatch{}, fmt.Errorf("reminder batch and recipient set disagree")
	}
	for _, recipient := range recipients {
		if recipient.AppID != row.AppID || recipient.TemplateID != row.TemplateID {
			return appnotification.ReminderBatch{}, fmt.Errorf("reminder batch recipient configuration disagrees")
		}
		batch.Recipients = append(batch.Recipients, appnotification.ReminderRecipientIdentity{
			UserID: recipient.UserID, LoginIdentityID: recipient.LoginIdentityID,
		})
	}
	slices.SortFunc(batch.Recipients, func(a, b appnotification.ReminderRecipientIdentity) int {
		if a.UserID < b.UserID {
			return -1
		}
		if a.UserID > b.UserID {
			return 1
		}
		if a.LoginIdentityID < b.LoginIdentityID {
			return -1
		}
		if a.LoginIdentityID > b.LoginIdentityID {
			return 1
		}
		return 0
	})
	return batch, nil
}

func (s *ReminderBatchLedger) FindBatch(
	ctx context.Context, key appnotification.ReminderBatchKey,
) (appnotification.ReminderBatch, bool, error) {
	batch, err := s.ReadBatch(ctx, key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return appnotification.ReminderBatch{}, false, nil
	}
	return batch, err == nil, err
}

func (s *ReminderBatchLedger) batchQuery(ctx context.Context, key appnotification.ReminderBatchKey) *gorm.DB {
	return s.db.WithContext(ctx).Model(&reminderBatchPO{}).Where(
		"org_id = ? AND task_id = ? AND opening_event_id = ? AND schedule_revision = ? AND reminder_version = ?",
		key.OrgID, key.TaskID, key.OpeningEventID, key.ScheduleRevision, key.ReminderVersion,
	)
}

func (s *ReminderBatchLedger) validateBatch(
	key appnotification.ReminderBatchKey, appID, templateID string, now time.Time,
) error {
	if err := s.validateBatchKey(key); err != nil {
		return err
	}
	if err := validateIdentity(appID, 128, "AppID"); err != nil {
		return err
	}
	if err := validateIdentity(templateID, 128, "template ID"); err != nil {
		return err
	}
	if now.IsZero() {
		return fmt.Errorf("reminder batch time is required")
	}
	return nil
}

func (s *ReminderBatchLedger) validateBatchKey(key appnotification.ReminderBatchKey) error {
	if s == nil || s.db == nil || key.OrgID <= 0 || key.ReminderVersion == 0 {
		return fmt.Errorf("reminder batch ledger, organization, and version are required")
	}
	if err := validateIdentity(key.TaskID, 64, "task ID"); err != nil {
		return err
	}
	return validateIdentity(key.OpeningEventID, 64, "opening event ID")
}

var _ appnotification.ReminderBatchLedger = (*ReminderBatchLedger)(nil)
