package eventoutbox

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/FangcunMount/component-base/pkg/event"
)

type OutboxPO struct {
	ID                    uint64     `gorm:"primaryKey;autoIncrement"`
	EventID               string     `gorm:"column:event_id;size:64;not null;uniqueIndex:uk_event_id"`
	EventType             string     `gorm:"column:event_type;size:128;not null"`
	AggregateType         string     `gorm:"column:aggregate_type;size:64;not null"`
	AggregateID           string     `gorm:"column:aggregate_id;size:64;not null"`
	OrgID                 *int64     `gorm:"column:org_id"`
	TopicName             string     `gorm:"column:topic_name;size:128;not null"`
	PayloadJSON           string     `gorm:"column:payload_json;type:longtext;not null"`
	Status                string     `gorm:"column:status;size:32;not null;index:idx_status_next_attempt_at,priority:1"`
	AttemptCount          int        `gorm:"column:attempt_count;not null;default:0"`
	RetryDisposition      *string    `gorm:"column:retry_disposition;size:32"`
	NextAttemptAt         time.Time  `gorm:"column:next_attempt_at;not null;index:idx_status_next_attempt_at,priority:2"`
	LastError             *string    `gorm:"column:last_error;type:text"`
	LastErrorKind         *string    `gorm:"column:last_error_kind;size:32"`
	ManualReplayRequestID *string    `gorm:"column:manual_replay_request_id;size:64"`
	CreatedAt             time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt             time.Time  `gorm:"column:updated_at;not null"`
	PublishedAt           *time.Time `gorm:"column:published_at"`
}

func (OutboxPO) TableName() string {
	return "domain_event_outbox"
}

type Store struct {
	db                 *gorm.DB
	publishingStaleFor time.Duration
	topicResolver      eventcatalog.TopicResolver
	priorityTiers      [][]string
}

type StoreOption func(*Store)

func WithPriorityTiers(tiers [][]string) StoreOption {
	return func(s *Store) {
		if len(tiers) == 0 {
			return
		}
		s.priorityTiers = tiers
	}
}

func NewStoreWithTopicResolver(db *gorm.DB, resolver eventcatalog.TopicResolver, opts ...StoreOption) *Store {
	if resolver == nil {
		resolver = eventcatalog.NewCatalog(nil)
	}
	store := &Store{
		db:                 db,
		publishingStaleFor: outboxcore.DefaultPublishingStaleFor,
		topicResolver:      resolver,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(store)
		}
	}
	return store
}

func (s *Store) Stage(ctx context.Context, events ...event.DomainEvent) error {
	tx, err := mysql.RequireTx(ctx)
	if err != nil {
		return err
	}
	return s.stageWithDB(tx, events)
}

func (s *Store) StageAt(ctx context.Context, dueAt time.Time, events ...event.DomainEvent) error {
	tx, err := mysql.RequireTx(ctx)
	if err != nil {
		return err
	}
	rows, err := s.buildRows(events)
	if err != nil {
		return err
	}
	for _, row := range rows {
		row.NextAttemptAt = dueAt
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

func (s *Store) stageWithDB(tx *gorm.DB, events []event.DomainEvent) error {
	if tx == nil {
		return mysql.ErrActiveTransactionRequired
	}
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
			OrgID:         outboxcore.OrgIDFromPayloadJSON(record.PayloadJSON),
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

	rows := make([]*OutboxPO, 0, limit)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		remaining := limit
		for _, tier := range s.priorityTiers {
			if remaining <= 0 {
				break
			}
			batch, err := s.claimDueBatch(tx, remaining, now, tier)
			if err != nil {
				return err
			}
			rows = append(rows, batch...)
			remaining = limit - len(rows)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.pendingFromRows(ctx, rows)
}

func (s *Store) claimDueBatch(tx *gorm.DB, limit int, now time.Time, eventTypes []string) ([]*OutboxPO, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []*OutboxPO
	query := s.dueEventsSelectionQuery(tx, now)
	if len(eventTypes) > 0 {
		query = query.Where("event_type IN ?", eventTypes)
	}
	if err := query.Order("created_at ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	if err := tx.Model(&OutboxPO{}).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{
			"status":     outboxcore.StatusPublishing,
			"updated_at": now,
		}).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Store) pendingFromRows(ctx context.Context, rows []*OutboxPO) ([]outboxport.PendingEvent, error) {
	claimed := make([]outboxport.PendingEvent, 0, len(rows))
	for _, row := range rows {
		pending, err := outboxcore.DecodePendingEvent(row.EventID, row.PayloadJSON)
		if err != nil {
			transition := outboxcore.NewDecodeFailureTransition(err, time.Now())
			_ = s.markPermanentFailure(ctx, row.EventID, transition.LastError, "encoding", time.Now())
			continue
		}
		claimed = append(claimed, pending)
	}
	return claimed, nil
}

func (s *Store) markPermanentFailure(ctx context.Context, eventID, lastError, errorKind string, failedAt time.Time) error {
	result := s.db.WithContext(ctx).Model(&OutboxPO{}).Where("event_id = ?", eventID).Updates(map[string]interface{}{
		"status": outboxcore.StatusFailed, "last_error": lastError, "last_error_kind": errorKind,
		"attempt_count": gorm.Expr("attempt_count + 1"), "retry_disposition": string(retrygovernance.DispositionManualRequired),
		"next_attempt_at": failedAt, "updated_at": failedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("outbox event %q not found", eventID)
	}
	return nil
}

func (s *Store) dueEventsSelectionQuery(tx *gorm.DB, now time.Time) *gorm.DB {
	staleBefore := now.Add(-s.publishingStaleFor)
	return tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where(
			"(status = ? AND next_attempt_at <= ?) OR (status = ? AND next_attempt_at <= ?) OR (status = ? AND updated_at <= ?)",
			outboxcore.StatusPending, now,
			outboxcore.StatusFailed, now,
			outboxcore.StatusPublishing, staleBefore,
		).
		Where("retry_disposition IS NULL OR retry_disposition <> ?", retrygovernance.DispositionManualRequired)
}

func (s *Store) MarkEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	transition := outboxcore.NewPublishedTransition(publishedAt)
	result := s.db.WithContext(ctx).Model(&OutboxPO{}).
		Where("event_id = ?", eventID).
		Updates(map[string]interface{}{
			"status":            transition.Status,
			"published_at":      transition.PublishedAt,
			"retry_disposition": nil,
			"updated_at":        transition.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("outbox event %q not found", eventID)
	}
	return nil
}

func (s *Store) MarkEventsFailedGoverned(ctx context.Context, failures []outboxport.FailedMark, failedAt time.Time) ([]outboxport.GovernedFailedMark, error) {
	if s == nil || s.db == nil || len(failures) == 0 {
		return nil, nil
	}
	ordered := append([]outboxport.FailedMark(nil), failures...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].EventID < ordered[j].EventID })
	results := make([]outboxport.GovernedFailedMark, 0, len(ordered))
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, failure := range ordered {
			var row OutboxPO
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("event_id = ?", failure.EventID).First(&row).Error; err != nil {
				return err
			}
			nextCount := row.AttemptCount + 1
			decision := retrygovernance.OutboxPolicy().DecideFailureForKey(true, nextCount, failedAt, failure.EventID)
			nextAt := failedAt
			if decision.NextAttemptAt != nil {
				nextAt = *decision.NextAttemptAt
			}
			if err := tx.Model(&OutboxPO{}).Where("id = ? AND attempt_count = ?", row.ID, row.AttemptCount).Updates(map[string]interface{}{
				"status": outboxcore.StatusFailed, "last_error": failure.LastError,
				"last_error_kind": "publish", "attempt_count": nextCount,
				"retry_disposition": string(decision.Disposition), "next_attempt_at": nextAt,
				"updated_at": failedAt,
			}).Error; err != nil {
				return err
			}
			results = append(results, outboxport.GovernedFailedMark{EventID: failure.EventID, EventType: failure.EventType, AttemptCount: nextCount, Disposition: decision.Disposition, NextAttemptAt: decision.NextAttemptAt})
		}
		return nil
	})
	return results, err
}

func (s *Store) MarkEventFailed(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error {
	transition := outboxcore.NewFailedTransition(lastError, nextAttemptAt, time.Now())
	result := s.db.WithContext(ctx).Model(&OutboxPO{}).
		Where("event_id = ?", eventID).
		Updates(map[string]interface{}{
			"status":          transition.Status,
			"last_error":      transition.LastError,
			"next_attempt_at": transition.NextAttemptAt,
			"updated_at":      transition.UpdatedAt,
			"attempt_count":   gorm.Expr("attempt_count + ?", transition.AttemptIncrement),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("outbox event %q not found", eventID)
	}
	return nil
}

func (s *Store) OutboxStatusSnapshot(ctx context.Context, now time.Time) (outboxport.StatusSnapshot, error) {
	if s == nil || s.db == nil {
		return outboxcore.BuildStatusSnapshot("assessment-mysql-outbox", now, nil), nil
	}

	statuses := outboxcore.UnfinishedStatuses()
	observations := make([]outboxcore.StatusObservation, 0, len(statuses))
	for _, status := range statuses {
		var count int64
		if err := outboxStatusCountQuery(s.db.WithContext(ctx), status).Count(&count).Error; err != nil {
			return outboxport.StatusSnapshot{}, err
		}
		var oldest OutboxPO
		var oldestCreatedAt *time.Time
		if count > 0 {
			if err := outboxOldestStatusQuery(s.db.WithContext(ctx), status).Find(&oldest).Error; err != nil {
				return outboxport.StatusSnapshot{}, err
			}
			oldestCreatedAt = &oldest.CreatedAt
		}
		observations = append(observations, outboxcore.StatusObservation{
			Status:          status,
			Count:           count,
			OldestCreatedAt: oldestCreatedAt,
		})
	}
	return outboxcore.BuildStatusSnapshot("assessment-mysql-outbox", now, observations), nil
}

func outboxStatusCountQuery(db *gorm.DB, status string) *gorm.DB {
	return db.Model(&OutboxPO{}).Where("status = ?", status)
}

func outboxOldestStatusQuery(db *gorm.DB, status string) *gorm.DB {
	return db.Where("status = ?", status).
		Order("created_at ASC").
		Limit(1)
}
