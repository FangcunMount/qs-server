package eventoutbox

import (
	"context"
	"fmt"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	statusPending    = "pending"
	statusPublishing = "publishing"
	statusPublished  = "published"
	statusFailed     = "failed"

	defaultPublishingStaleFor = 1 * time.Minute
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
		publishingStaleFor: defaultPublishingStaleFor,
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
	if len(events) == 0 {
		return nil, nil
	}

	now := time.Now()
	rows := make([]*OutboxPO, 0, len(events))
	for _, evt := range events {
		topicName, ok := s.topicResolver.GetTopicForEvent(evt.EventType())
		if !ok {
			return nil, fmt.Errorf("event %q not found in event config", evt.EventType())
		}
		payload, err := eventcodec.EncodeDomainEvent(evt)
		if err != nil {
			return nil, err
		}
		rows = append(rows, &OutboxPO{
			EventID:       evt.EventID(),
			EventType:     evt.EventType(),
			AggregateType: evt.AggregateType(),
			AggregateID:   evt.AggregateID(),
			TopicName:     topicName,
			PayloadJSON:   string(payload),
			Status:        statusPending,
			AttemptCount:  0,
			NextAttemptAt: now,
			CreatedAt:     now,
			UpdatedAt:     now,
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
				statusPending, now,
				statusFailed, now,
				statusPublishing, staleBefore,
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
				"status":     statusPublishing,
				"updated_at": now,
			}).Error
	})
	if err != nil {
		return nil, err
	}

	claimed := make([]outboxport.PendingEvent, 0, len(rows))
	for _, row := range rows {
		evt, err := eventcodec.DecodeDomainEvent([]byte(row.PayloadJSON))
		if err != nil {
			_ = s.MarkEventFailed(ctx, row.EventID, fmt.Sprintf("decode outbox payload: %v", err), time.Now().Add(10*time.Second))
			continue
		}
		claimed = append(claimed, outboxport.PendingEvent{
			EventID: row.EventID,
			Event:   evt,
		})
	}

	return claimed, nil
}

func (s *Store) MarkEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	return s.db.WithContext(ctx).Model(&OutboxPO{}).
		Where("event_id = ?", eventID).
		Updates(map[string]interface{}{
			"status":       statusPublished,
			"published_at": publishedAt,
			"updated_at":   publishedAt,
		}).Error
}

func (s *Store) MarkEventFailed(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error {
	return s.db.WithContext(ctx).Model(&OutboxPO{}).
		Where("event_id = ?", eventID).
		Updates(map[string]interface{}{
			"status":          statusFailed,
			"last_error":      lastError,
			"next_attempt_at": nextAttemptAt,
			"updated_at":      time.Now(),
			"attempt_count":   gorm.Expr("attempt_count + 1"),
		}).Error
}
