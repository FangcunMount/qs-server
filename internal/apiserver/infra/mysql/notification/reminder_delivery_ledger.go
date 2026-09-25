package notification

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	appnotification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	"gorm.io/gorm"
)

type reminderDeliveryPO struct {
	ID                    uint64     `gorm:"column:id;primaryKey"`
	OrgID                 int64      `gorm:"column:org_id"`
	TaskID                string     `gorm:"column:task_id"`
	OpeningEventID        string     `gorm:"column:opening_event_id"`
	ScheduleRevision      uint32     `gorm:"column:schedule_revision"`
	UserID                string     `gorm:"column:user_id"`
	LoginIdentityID       string     `gorm:"column:login_identity_id"`
	AppID                 string     `gorm:"column:app_id"`
	TemplateID            string     `gorm:"column:template_id"`
	ReminderVersion       uint32     `gorm:"column:reminder_version"`
	State                 string     `gorm:"column:state"`
	ClaimToken            *string    `gorm:"column:claim_token"`
	LeaseUntil            *time.Time `gorm:"column:lease_until"`
	ExternalCallStartedAt *time.Time `gorm:"column:external_call_started_at"`
	PlatformMessageID     *string    `gorm:"column:platform_message_id"`
	ResolutionCode        string     `gorm:"column:resolution_code"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at"`
}

func (reminderDeliveryPO) TableName() string { return "task_opened_reminder_delivery" }

type ReminderDeliveryLedger struct{ db *gorm.DB }

func NewReminderDeliveryLedger(db *gorm.DB) *ReminderDeliveryLedger {
	return &ReminderDeliveryLedger{db: db}
}

func (s *ReminderDeliveryLedger) EnsurePending(
	ctx context.Context, key appnotification.ReminderDeliveryKey, userID string, now time.Time,
) (appnotification.ReminderDelivery, error) {
	if err := s.validate(key, now); err != nil {
		return appnotification.ReminderDelivery{}, err
	}
	if err := validateIdentity(userID, 64, "user ID"); err != nil {
		return appnotification.ReminderDelivery{}, err
	}
	// The unique index owns concurrent deduplication. Never reset an existing
	// state or update its frozen recipient when the broker redelivers.
	err := s.db.WithContext(ctx).Exec(`INSERT INTO task_opened_reminder_delivery
		(org_id,task_id,opening_event_id,schedule_revision,user_id,login_identity_id,
		 app_id,template_id,reminder_version,state,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,'pending',?,?)
		ON DUPLICATE KEY UPDATE id=id`,
		key.OrgID, key.TaskID, key.OpeningEventID, key.ScheduleRevision, userID,
		key.LoginIdentityID, key.AppID, key.TemplateID, key.ReminderVersion, now, now,
	).Error
	if err != nil {
		return appnotification.ReminderDelivery{}, fmt.Errorf("ensure reminder delivery: %w", err)
	}
	delivery, err := s.Read(ctx, key)
	if err != nil {
		return appnotification.ReminderDelivery{}, err
	}
	if delivery.UserID != userID {
		return appnotification.ReminderDelivery{}, fmt.Errorf("reminder recipient identity conflict")
	}
	return delivery, nil
}

func (s *ReminderDeliveryLedger) Claim(
	ctx context.Context, key appnotification.ReminderDeliveryKey, lease time.Duration, now time.Time,
) (string, bool, error) {
	if err := s.validate(key, now); err != nil {
		return "", false, err
	}
	if lease <= 0 || lease > 10*time.Minute {
		return "", false, fmt.Errorf("reminder claim lease must be positive and at most ten minutes")
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", false, fmt.Errorf("generate reminder claim token: %w", err)
	}
	token := hex.EncodeToString(random[:])
	result := s.whereKey(ctx, key).Where(
		"state = ? OR (state = ? AND lease_until <= ? AND external_call_started_at IS NULL)",
		appnotification.ReminderPending, appnotification.ReminderClaimed, now,
	).Updates(map[string]any{
		"state": appnotification.ReminderClaimed, "claim_token": token,
		"lease_until": now.Add(lease), "updated_at": now,
	})
	if result.Error != nil {
		return "", false, fmt.Errorf("claim reminder delivery: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return "", false, nil
	}
	return token, true, nil
}

func (s *ReminderDeliveryLedger) BeginExternalCall(
	ctx context.Context, key appnotification.ReminderDeliveryKey, token string, now time.Time,
) (bool, error) {
	if err := s.validateClaim(key, token, now); err != nil {
		return false, err
	}
	return s.transition(ctx, key,
		"state = ? AND claim_token = ? AND lease_until > ? AND external_call_started_at IS NULL",
		[]any{appnotification.ReminderClaimed, token, now},
		map[string]any{"state": appnotification.ReminderSending, "external_call_started_at": now, "lease_until": nil, "updated_at": now},
	)
}

func (s *ReminderDeliveryLedger) Confirm(
	ctx context.Context, key appnotification.ReminderDeliveryKey, token, platformMessageID string, now time.Time,
) (bool, error) {
	if err := s.validateClaim(key, token, now); err != nil {
		return false, err
	}
	if err := validateIdentity(platformMessageID, 64, "platform message ID"); err != nil {
		return false, err
	}
	return s.transition(ctx, key,
		"state = ? AND claim_token = ? AND external_call_started_at IS NOT NULL AND platform_message_id IS NULL",
		[]any{appnotification.ReminderSending, token},
		map[string]any{"state": appnotification.ReminderConfirmed, "platform_message_id": platformMessageID, "updated_at": now},
	)
}

func (s *ReminderDeliveryLedger) MarkUnknown(
	ctx context.Context, key appnotification.ReminderDeliveryKey, token, resolutionCode string, now time.Time,
) (bool, error) {
	if err := s.validateClaim(key, token, now); err != nil {
		return false, err
	}
	if err := validateResolutionCode(resolutionCode); err != nil {
		return false, err
	}
	return s.transition(ctx, key,
		"state = ? AND claim_token = ? AND external_call_started_at IS NOT NULL",
		[]any{appnotification.ReminderSending, token},
		map[string]any{"state": appnotification.ReminderManualRequired, "resolution_code": resolutionCode, "updated_at": now},
	)
}

func (s *ReminderDeliveryLedger) ReleaseUnsent(
	ctx context.Context, key appnotification.ReminderDeliveryKey, token string, now time.Time,
) (bool, error) {
	if err := s.validateClaim(key, token, now); err != nil {
		return false, err
	}
	return s.transition(ctx, key,
		"state = ? AND claim_token = ? AND external_call_started_at IS NULL",
		[]any{appnotification.ReminderClaimed, token},
		map[string]any{"state": appnotification.ReminderPending, "claim_token": nil, "lease_until": nil, "updated_at": now},
	)
}

func (s *ReminderDeliveryLedger) SuppressUnsent(
	ctx context.Context, key appnotification.ReminderDeliveryKey, token, resolutionCode string, now time.Time,
) (bool, error) {
	if err := s.validateClaim(key, token, now); err != nil {
		return false, err
	}
	if err := validateResolutionCode(resolutionCode); err != nil {
		return false, err
	}
	return s.transition(ctx, key,
		"state = ? AND claim_token = ? AND external_call_started_at IS NULL",
		[]any{appnotification.ReminderClaimed, token},
		map[string]any{"state": appnotification.ReminderSuppressed, "lease_until": nil, "resolution_code": resolutionCode, "updated_at": now},
	)
}

func (s *ReminderDeliveryLedger) Read(
	ctx context.Context, key appnotification.ReminderDeliveryKey,
) (appnotification.ReminderDelivery, error) {
	if err := s.validateKey(key); err != nil {
		return appnotification.ReminderDelivery{}, err
	}
	var row reminderDeliveryPO
	if err := s.whereKey(ctx, key).First(&row).Error; err != nil {
		return appnotification.ReminderDelivery{}, fmt.Errorf("read reminder delivery: %w", err)
	}
	return toDelivery(row), nil
}

// ListNeedsReview exposes unresolved sends without interpreting an old
// "sending" marker as permission to call the platform again.
func (s *ReminderDeliveryLedger) ListNeedsReview(
	ctx context.Context, orgID int64, sendingOlderThan time.Time, limit int,
) ([]appnotification.ReminderDelivery, error) {
	if s == nil || s.db == nil || orgID <= 0 || sendingOlderThan.IsZero() || limit <= 0 || limit > 100 {
		return nil, fmt.Errorf("valid organization, review cutoff, and limit are required")
	}
	var rows []reminderDeliveryPO
	err := s.db.WithContext(ctx).Where(
		"org_id = ? AND ((state = ? AND updated_at <= ?) OR state = ?)",
		orgID, appnotification.ReminderSending, sendingOlderThan, appnotification.ReminderManualRequired,
	).Order("updated_at ASC, id ASC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list reminder deliveries needing review: %w", err)
	}
	deliveries := make([]appnotification.ReminderDelivery, 0, len(rows))
	for _, row := range rows {
		deliveries = append(deliveries, toDelivery(row))
	}
	return deliveries, nil
}

func toDelivery(row reminderDeliveryPO) appnotification.ReminderDelivery {
	return appnotification.ReminderDelivery{
		Key: appnotification.ReminderDeliveryKey{
			OrgID: row.OrgID, TaskID: row.TaskID, OpeningEventID: row.OpeningEventID,
			ScheduleRevision: row.ScheduleRevision, LoginIdentityID: row.LoginIdentityID,
			AppID: row.AppID, TemplateID: row.TemplateID, ReminderVersion: row.ReminderVersion,
		},
		UserID: row.UserID, State: appnotification.ReminderDeliveryState(row.State),
		ClaimToken: value(row.ClaimToken), LeaseUntil: row.LeaseUntil,
		ExternalCallStartedAt: row.ExternalCallStartedAt,
		PlatformMessageID:     value(row.PlatformMessageID), ResolutionCode: row.ResolutionCode,
		UpdatedAt: row.UpdatedAt,
	}
}

func (s *ReminderDeliveryLedger) transition(
	ctx context.Context, key appnotification.ReminderDeliveryKey,
	condition string, args []any, changes map[string]any,
) (bool, error) {
	result := s.whereKey(ctx, key).Where(condition, args...).Updates(changes)
	if result.Error != nil {
		return false, fmt.Errorf("transition reminder delivery: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (s *ReminderDeliveryLedger) whereKey(ctx context.Context, key appnotification.ReminderDeliveryKey) *gorm.DB {
	return s.db.WithContext(ctx).Model(&reminderDeliveryPO{}).Where(
		"org_id = ? AND task_id = ? AND opening_event_id = ? AND schedule_revision = ? AND login_identity_id = ? AND app_id = ? AND reminder_version = ?",
		key.OrgID, key.TaskID, key.OpeningEventID, key.ScheduleRevision,
		key.LoginIdentityID, key.AppID, key.ReminderVersion,
	)
}

func (s *ReminderDeliveryLedger) validate(key appnotification.ReminderDeliveryKey, now time.Time) error {
	if err := s.validateKey(key); err != nil {
		return err
	}
	if now.IsZero() {
		return fmt.Errorf("reminder transition time is required")
	}
	return nil
}

func (s *ReminderDeliveryLedger) validateClaim(key appnotification.ReminderDeliveryKey, token string, now time.Time) error {
	if err := s.validate(key, now); err != nil {
		return err
	}
	return validateIdentity(token, 64, "claim token")
}

func (s *ReminderDeliveryLedger) validateKey(key appnotification.ReminderDeliveryKey) error {
	if s == nil || s.db == nil || key.OrgID <= 0 || key.ReminderVersion == 0 {
		return fmt.Errorf("reminder ledger, organization, and version are required")
	}
	for _, field := range []struct {
		value string
		limit int
		name  string
	}{
		{key.TaskID, 64, "task ID"}, {key.OpeningEventID, 64, "opening event ID"},
		{key.LoginIdentityID, 64, "login identity ID"}, {key.AppID, 128, "AppID"},
		{key.TemplateID, 128, "template ID"},
	} {
		if err := validateIdentity(field.value, field.limit, field.name); err != nil {
			return err
		}
	}
	return nil
}

func validateIdentity(value string, limit int, name string) error {
	if value == "" || strings.TrimSpace(value) != value || len(value) > limit {
		return fmt.Errorf("valid %s is required", name)
	}
	return nil
}

func validateResolutionCode(value string) error {
	if value == "" || len(value) > 64 {
		return fmt.Errorf("reminder resolution code is required")
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return fmt.Errorf("reminder resolution code must be machine-readable")
		}
	}
	return nil
}

func value(raw *string) string {
	if raw == nil {
		return ""
	}
	return *raw
}

var _ appnotification.ReminderDeliveryLedger = (*ReminderDeliveryLedger)(nil)
