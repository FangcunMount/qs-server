package eventoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/FangcunMount/qs-server/pkg/event"
)

type OutboxPO struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement"`
	EventID       string     `gorm:"column:event_id;size:64;not null;uniqueIndex:uk_event_id"`
	EventType     string     `gorm:"column:event_type;size:128;not null"`
	AggregateType string     `gorm:"column:aggregate_type;size:64;not null"`
	AggregateID   string     `gorm:"column:aggregate_id;size:64;not null"`
	TopicName     string     `gorm:"column:topic_name;size:128;not null"`
	PayloadJSON   string     `gorm:"column:payload_json;type:longtext;not null"`
	Status        string     `gorm:"column:status;size:32;not null;index:idx_status_next_attempt_at,priority:1"`
	AttemptCount  int        `gorm:"column:attempt_count;not null;default:0"`
	NextAttemptAt time.Time  `gorm:"column:next_attempt_at;not null;index:idx_status_next_attempt_at,priority:2"`
	LastError     *string    `gorm:"column:last_error;type:text"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;not null"`
	PublishedAt   *time.Time `gorm:"column:published_at"`
}

func (OutboxPO) TableName() string {
	return "domain_event_outbox"
}

type Store struct {
	db                 *gorm.DB
	publishingStaleFor time.Duration
	topicResolver      eventcatalog.TopicResolver
}

func NewStore(db *gorm.DB) *Store {
	return NewStoreWithTopicResolver(db, eventcatalog.NewCatalog(nil))
}

func NewStoreWithTopicResolver(db *gorm.DB, resolver eventcatalog.TopicResolver) *Store {
	if resolver == nil {
		resolver = eventcatalog.NewCatalog(nil)
	}
	return &Store{
		db:                 db,
		publishingStaleFor: outboxcore.DefaultPublishingStaleFor,
		topicResolver:      resolver,
	}
}

func (s *Store) StageEventsTx(tx *gorm.DB, events []event.DomainEvent) error {
	rows, err := s.buildRows(events)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

func (s *Store) buildRows(events []event.DomainEvent) ([]*OutboxPO, error) {
	return s.buildRowsAt(events, time.Now())
}

func (s *Store) buildRowsAt(events []event.DomainEvent, now time.Time) ([]*OutboxPO, error) {
	records, err := outboxcore.BuildRecords(outboxcore.BuildRecordsOptions{
		Events:   events,
		Resolver: s.topicResolver,
		Now:      now,
	})
	if err != nil {
		return nil, err
	}
	rows := make([]*OutboxPO, 0, len(records))
	for _, record := range records {
		rows = append(rows, &OutboxPO{
			EventID:       record.EventID,
			EventType:     record.EventType,
			AggregateType: record.AggregateType,
			AggregateID:   record.AggregateID,
			TopicName:     record.TopicName,
			PayloadJSON:   record.PayloadJSON,
			Status:        record.Status,
			AttemptCount:  record.AttemptCount,
			NextAttemptAt: record.NextAttemptAt,
			CreatedAt:     record.CreatedAt,
			UpdatedAt:     record.UpdatedAt,
		})
	}
	return rows, nil
}

func (s *Store) ClaimDueEvents(ctx context.Context, limit int, now time.Time) ([]outboxport.PendingEvent, error) {
	if s == nil || s.db == nil || limit <= 0 {
		return nil, nil
	}

	var rows []*OutboxPO
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		staleBefore := now.Add(-s.publishingStaleFor)
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(
				"(status = ? AND next_attempt_at <= ?) OR (status = ? AND next_attempt_at <= ?) OR (status = ? AND updated_at <= ?)",
				outboxcore.StatusPending, now,
				outboxcore.StatusFailed, now,
				outboxcore.StatusPublishing, staleBefore,
			).
			Order("created_at ASC").
			Limit(limit).
			Find(&rows).Error; err != nil {
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

	claimed := make([]outboxport.PendingEvent, 0, len(rows))
	for _, row := range rows {
		pending, err := outboxcore.DecodePendingEvent(row.EventID, row.PayloadJSON)
		if err != nil {
			transition := outboxcore.NewDecodeFailureTransition(err, time.Now())
			_ = s.MarkEventFailed(ctx, row.EventID, transition.LastError, transition.NextAttemptAt)
			continue
		}
		claimed = append(claimed, pending)
	}

	return claimed, nil
}

func (s *Store) MarkEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	transition := outboxcore.NewPublishedTransition(publishedAt)
	return s.db.WithContext(ctx).Model(&OutboxPO{}).
		Where("event_id = ?", eventID).
		Updates(map[string]interface{}{
			"status":       transition.Status,
			"published_at": transition.PublishedAt,
			"updated_at":   transition.UpdatedAt,
		}).Error
}

func (s *Store) MarkEventFailed(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error {
	transition := outboxcore.NewFailedTransition(lastError, nextAttemptAt, time.Now())
	return s.db.WithContext(ctx).Model(&OutboxPO{}).
		Where("event_id = ?", eventID).
		Updates(map[string]interface{}{
			"status":          transition.Status,
			"last_error":      transition.LastError,
			"next_attempt_at": transition.NextAttemptAt,
			"updated_at":      transition.UpdatedAt,
			"attempt_count":   gorm.Expr("attempt_count + ?", transition.AttemptIncrement),
		}).Error
}
