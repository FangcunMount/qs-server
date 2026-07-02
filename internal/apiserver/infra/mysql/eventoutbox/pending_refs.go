package eventoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

func (s *Store) ListPendingEventRefs(ctx context.Context, limit int, now time.Time) ([]outboxport.PendingEventRef, error) {
	if s == nil || s.db == nil || limit <= 0 {
		return nil, nil
	}
	rows := make([]OutboxPO, 0)
	err := s.db.WithContext(ctx).
		Select("event_id", "event_type", "next_attempt_at", "created_at").
		Where("status IN ? AND next_attempt_at <= ?", []string{outboxcore.StatusPending, outboxcore.StatusFailed}, now).
		Order("created_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	refs := make([]outboxport.PendingEventRef, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, outboxport.PendingEventRef{
			EventID:       row.EventID,
			EventType:     row.EventType,
			NextAttemptAt: row.NextAttemptAt,
			CreatedAt:     row.CreatedAt,
		})
	}
	return refs, nil
}
