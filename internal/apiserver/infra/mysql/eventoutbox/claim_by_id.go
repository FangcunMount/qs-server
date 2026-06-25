package eventoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"gorm.io/gorm"
)

func (s *Store) ClaimEventsByIDs(ctx context.Context, eventIDs []string, now time.Time) ([]outboxport.PendingEvent, error) {
	if s == nil || s.db == nil || len(eventIDs) == 0 {
		return nil, nil
	}

	rows := make([]*OutboxPO, 0, len(eventIDs))
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := s.dueEventsSelectionQuery(tx, now).Where("event_id IN ?", eventIDs)
		if err := query.Order("created_at ASC").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uint64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		return tx.Model(&OutboxPO{}).
			Where("id IN ?", ids).
			Updates(map[string]interface{}{
				"status":     outboxcore.StatusPublishing,
				"updated_at": now,
			}).Error
	})
	if err != nil {
		return nil, err
	}
	return s.pendingFromRows(ctx, rows)
}
