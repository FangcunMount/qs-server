package eventoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

type eventTypeStatusRow struct {
	EventType string    `gorm:"column:event_type"`
	Status    string    `gorm:"column:status"`
	Count     int64     `gorm:"column:count"`
	Oldest    time.Time `gorm:"column:oldest"`
}

func (s *Store) OutboxStatusByEventType(ctx context.Context, now time.Time) ([]outboxport.EventTypeStatusBucket, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	_ = now
	rows := make([]eventTypeStatusRow, 0)
	err := s.db.WithContext(ctx).
		Model(&OutboxPO{}).
		Select("event_type, status, COUNT(*) AS count, MIN(created_at) AS oldest").
		Where("status IN ?", outboxcore.UnfinishedStatuses()).
		Group("event_type, status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	buckets := make([]outboxport.EventTypeStatusBucket, 0, len(rows))
	for _, row := range rows {
		oldest := row.Oldest
		buckets = append(buckets, outboxport.EventTypeStatusBucket{
			EventType:       row.EventType,
			Status:          row.Status,
			Count:           row.Count,
			OldestCreatedAt: &oldest,
		})
	}
	return buckets, nil
}
