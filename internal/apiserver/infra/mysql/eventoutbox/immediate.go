package eventoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"gorm.io/gorm"
)

func (s *Store) GetPublishableEvent(ctx context.Context, eventID string, now time.Time) (outboxport.PendingEvent, bool, error) {
	if s == nil || s.db == nil || eventID == "" {
		return outboxport.PendingEvent{}, false, nil
	}
	var row OutboxPO
	err := s.db.WithContext(ctx).
		Where("event_id = ? AND status IN ? AND next_attempt_at <= ?", eventID, []string{outboxcore.StatusPending, outboxcore.StatusFailed}, now).
		Where("retry_disposition IS NULL OR retry_disposition <> ?", "manual_required").
		First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return outboxport.PendingEvent{}, false, nil
	}
	if err != nil {
		return outboxport.PendingEvent{}, false, err
	}
	pending, err := outboxcore.DecodePendingEvent(row.EventID, row.PayloadJSON)
	if err != nil {
		_ = s.markPermanentFailure(ctx, row.EventID, "decode outbox payload: "+err.Error(), "encoding", now)
		return outboxport.PendingEvent{}, false, err
	}
	return pending, true, nil
}

func (s *Store) MarkEventsPublished(ctx context.Context, eventIDs []string, publishedAt time.Time) error {
	if s == nil || s.db == nil || len(eventIDs) == 0 {
		return nil
	}
	transition := outboxcore.NewPublishedTransition(publishedAt)
	return s.db.WithContext(ctx).Model(&OutboxPO{}).
		Where("event_id IN ?", eventIDs).
		Updates(map[string]interface{}{
			"status":            transition.Status,
			"published_at":      transition.PublishedAt,
			"retry_disposition": nil,
			"updated_at":        transition.UpdatedAt,
		}).Error
}

func (s *Store) MarkEventsFailed(ctx context.Context, failures []outboxport.FailedMark, nextAttemptAt time.Time) error {
	if s == nil || s.db == nil || len(failures) == 0 {
		return nil
	}
	now := time.Now()
	for _, failure := range failures {
		transition := outboxcore.NewFailedTransition(failure.LastError, nextAttemptAt, now)
		if err := s.db.WithContext(ctx).Model(&OutboxPO{}).
			Where("event_id = ?", failure.EventID).
			Updates(map[string]interface{}{
				"status":          transition.Status,
				"last_error":      transition.LastError,
				"next_attempt_at": transition.NextAttemptAt,
				"updated_at":      transition.UpdatedAt,
				"attempt_count":   gorm.Expr("attempt_count + ?", transition.AttemptIncrement),
			}).Error; err != nil {
			return err
		}
	}
	return nil
}
