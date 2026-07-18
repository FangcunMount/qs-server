package eventdelivery

import (
	"context"
	"fmt"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type deadLetterPO struct {
	ID               uint64     `gorm:"column:id;primaryKey"`
	MessageID        string     `gorm:"column:message_id"`
	EventID          *string    `gorm:"column:event_id"`
	OrgID            *int64     `gorm:"column:org_id"`
	DeliveryAttempts int        `gorm:"column:delivery_attempts"`
	PayloadJSON      string     `gorm:"column:payload_json"`
	LastError        *string    `gorm:"column:last_error"`
	RetryDisposition string     `gorm:"column:retry_disposition"`
	ReplayRequestID  *string    `gorm:"column:replay_request_id"`
	ReplayedAt       *time.Time `gorm:"column:replayed_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

func (deadLetterPO) TableName() string { return "event_delivery_dead_letter" }

type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) AuthorizeReplay(ctx context.Context, orgID int64, requestID string, targets []app.DeliveryReplayTarget, now time.Time) ([]app.AuthorizedDelivery, error) {
	if s == nil || s.db == nil || orgID <= 0 || requestID == "" || len(targets) == 0 || len(targets) > 100 {
		return nil, fmt.Errorf("invalid delivery replay authorization")
	}
	authorized := make([]app.AuthorizedDelivery, 0, len(targets))
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, target := range targets {
			if target.ID == 0 || target.ExpectedDeliveryAttempts < 1 {
				return fmt.Errorf("delivery dead-letter id and expected attempts are required")
			}
			var row deadLetterPO
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", target.ID).Error; err != nil {
				return err
			}
			if row.OrgID == nil || *row.OrgID != orgID {
				return fmt.Errorf("delivery dead-letter %d is outside organization scope", target.ID)
			}
			if row.RetryDisposition != "manual_required" || row.DeliveryAttempts != target.ExpectedDeliveryAttempts {
				return fmt.Errorf("delivery dead-letter %d replay state conflict", target.ID)
			}
			if err := tx.Model(&deadLetterPO{}).Where("id = ?", row.ID).Updates(map[string]any{
				"retry_disposition": "automatic", "replay_request_id": requestID, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			eventID := ""
			if row.EventID != nil {
				eventID = *row.EventID
			}
			authorized = append(authorized, app.AuthorizedDelivery{ID: row.ID, MessageID: row.MessageID, EventID: eventID, PayloadJSON: row.PayloadJSON})
		}
		return nil
	})
	return authorized, err
}

func (s *Store) CompleteReplay(ctx context.Context, id uint64, requestID string, now time.Time) error {
	result := s.db.WithContext(ctx).Model(&deadLetterPO{}).
		Where("id = ? AND retry_disposition = 'automatic' AND replay_request_id = ?", id, requestID).
		Updates(map[string]any{"retry_disposition": "terminal", "replayed_at": now, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("delivery dead-letter %d replay completion conflict", id)
	}
	return nil
}

func (s *Store) FailReplay(ctx context.Context, id uint64, requestID, lastError string, now time.Time) error {
	result := s.db.WithContext(ctx).Model(&deadLetterPO{}).
		Where("id = ? AND retry_disposition = 'automatic' AND replay_request_id = ?", id, requestID).
		Updates(map[string]any{"retry_disposition": "manual_required", "last_error": lastError, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("delivery dead-letter %d replay failure conflict", id)
	}
	return nil
}

var _ app.DeliveryReplayStore = (*Store)(nil)
