package eventoutbox

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrActiveSessionTransactionRequired = stderrors.New("active mongo session transaction required")

// OutboxPO stores domain events until they are durably published.
type OutboxPO struct {
	EventID       string    `bson:"event_id"`
	EventType     string    `bson:"event_type"`
	AggregateType string    `bson:"aggregate_type"`
	AggregateID   string    `bson:"aggregate_id"`
	TopicName     string    `bson:"topic_name"`
	PayloadJSON   string    `bson:"payload_json"`
	Status        string    `bson:"status"`
	AttemptCount  int       `bson:"attempt_count"`
	NextAttemptAt time.Time `bson:"next_attempt_at"`
	LastError     string    `bson:"last_error,omitempty"`
	CreatedAt     time.Time `bson:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at"`
	PublishedAt   time.Time `bson:"published_at,omitempty"`
}

func (OutboxPO) CollectionName() string {
	return "domain_event_outbox"
}

type Store struct {
	coll               *mongo.Collection
	publishingStaleFor time.Duration
	topicResolver      eventcatalog.TopicResolver
}

func NewStore(db *mongo.Database) (*Store, error) {
	return NewStoreWithTopicResolver(db, eventcatalog.NewCatalog(nil))
}

func NewStoreWithTopicResolver(db *mongo.Database, resolver eventcatalog.TopicResolver) (*Store, error) {
	if resolver == nil {
		resolver = eventcatalog.NewCatalog(nil)
	}
	store := &Store{
		coll:               db.Collection((&OutboxPO{}).CollectionName()),
		publishingStaleFor: outboxcore.DefaultPublishingStaleFor,
		topicResolver:      resolver,
	}
	if err := store.ensureIndexes(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) ensureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := s.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "event_id", Value: 1}},
			Options: options.Index().SetName("uk_event_id").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "status", Value: 1}, {Key: "next_attempt_at", Value: 1}},
			Options: options.Index().SetName("idx_status_next_attempt_at"),
		},
		{
			Keys: bson.D{
				{Key: "created_at", Value: 1},
				{Key: "next_attempt_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_pending_created_at_next_attempt_at").
				SetPartialFilterExpression(bson.M{"status": outboxcore.StatusPending}),
		},
		{
			Keys: bson.D{
				{Key: "next_attempt_at", Value: 1},
				{Key: "created_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_failed_next_attempt_at_created_at").
				SetPartialFilterExpression(bson.M{"status": outboxcore.StatusFailed}),
		},
		{
			Keys: bson.D{
				{Key: "updated_at", Value: 1},
				{Key: "created_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_publishing_updated_at_created_at").
				SetPartialFilterExpression(bson.M{"status": outboxcore.StatusPublishing}),
		},
	}); err != nil {
		return fmt.Errorf("create mongo outbox indexes: %w", err)
	}

	return nil
}

func (s *Store) Stage(ctx context.Context, events ...event.DomainEvent) error {
	txCtx, ok := ctx.(mongo.SessionContext)
	if !ok {
		return ErrActiveSessionTransactionRequired
	}
	return s.stageWithSession(txCtx, events)
}

// StageEventsTx stages events through an explicit Mongo session transaction.
// Deprecated: keep this only for existing repository-owned Mongo transactions.
func (s *Store) StageEventsTx(ctx mongo.SessionContext, events []event.DomainEvent) error {
	return s.stageWithSession(ctx, events)
}

func (s *Store) stageWithSession(ctx mongo.SessionContext, events []event.DomainEvent) error {
	docs, err := s.buildDocuments(events)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return nil
	}

	items := make([]interface{}, 0, len(docs))
	for _, doc := range docs {
		items = append(items, doc)
	}
	_, err = s.coll.InsertMany(ctx, items)
	return err
}

func (s *Store) buildDocuments(events []event.DomainEvent) ([]*OutboxPO, error) {
	return s.buildDocumentsAt(events, time.Now())
}

func (s *Store) buildDocumentsAt(events []event.DomainEvent, now time.Time) ([]*OutboxPO, error) {
	records, err := outboxcore.BuildRecords(outboxcore.BuildRecordsOptions{
		Events:   events,
		Resolver: s.topicResolver,
		Now:      now,
	})
	if err != nil {
		return nil, err
	}
	docs := make([]*OutboxPO, 0, len(records))
	for _, record := range records {
		docs = append(docs, &OutboxPO{
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
	return docs, nil
}

func (s *Store) ClaimDueEvents(ctx context.Context, limit int, now time.Time) ([]outboxport.PendingEvent, error) {
	if limit <= 0 {
		return nil, nil
	}

	claimed := make([]outboxport.PendingEvent, 0, limit)
	staleBefore := now.Add(-s.publishingStaleFor)

	// Recover a small amount of failed/stale work first so these rows do not
	// starve forever behind a large pending backlog.
	failedQuota := minInt(limit-len(claimed), 2)
	if failedQuota > 0 {
		items, err := s.claimDueByNextAttempt(ctx, outboxcore.StatusFailed, failedQuota, now)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, items...)
	}

	staleQuota := minInt(limit-len(claimed), 2)
	if staleQuota > 0 {
		items, err := s.claimStalePublishing(ctx, staleQuota, now, staleBefore)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, items...)
	}

	pendingQuota := limit - len(claimed)
	if pendingQuota > 0 {
		items, err := s.claimPending(ctx, pendingQuota, now)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, items...)
	}

	// If pending did not fill the batch, let failed rows use the remaining
	// capacity before touching stale publishing again.
	remaining := limit - len(claimed)
	if remaining > 0 {
		items, err := s.claimDueByNextAttempt(ctx, outboxcore.StatusFailed, remaining, now)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, items...)
	}

	remaining = limit - len(claimed)
	if remaining > 0 {
		items, err := s.claimStalePublishing(ctx, remaining, now, staleBefore)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, items...)
	}

	return claimed, nil
}

func (s *Store) claimPending(ctx context.Context, limit int, now time.Time) ([]outboxport.PendingEvent, error) {
	if limit <= 0 {
		return nil, nil
	}

	claimed := make([]outboxport.PendingEvent, 0, limit)
	for len(claimed) < limit {
		item, found, err := s.claimOne(ctx, bson.M{
			"status":          outboxcore.StatusPending,
			"next_attempt_at": bson.M{"$lte": now},
		}, bson.D{{Key: "created_at", Value: 1}}, now)
		if err != nil {
			return nil, err
		}
		if !found {
			break
		}
		claimed = append(claimed, item)
	}

	return claimed, nil
}

func (s *Store) claimDueByNextAttempt(ctx context.Context, status string, limit int, now time.Time) ([]outboxport.PendingEvent, error) {
	if limit <= 0 {
		return nil, nil
	}

	claimed := make([]outboxport.PendingEvent, 0, limit)
	for len(claimed) < limit {
		item, found, err := s.claimOne(ctx, bson.M{
			"status":          status,
			"next_attempt_at": bson.M{"$lte": now},
		}, bson.D{{Key: "next_attempt_at", Value: 1}, {Key: "created_at", Value: 1}}, now)
		if err != nil {
			return nil, err
		}
		if !found {
			break
		}
		claimed = append(claimed, item)
	}

	return claimed, nil
}

func (s *Store) claimStalePublishing(ctx context.Context, limit int, now, staleBefore time.Time) ([]outboxport.PendingEvent, error) {
	if limit <= 0 {
		return nil, nil
	}

	claimed := make([]outboxport.PendingEvent, 0, limit)
	for len(claimed) < limit {
		item, found, err := s.claimOne(ctx, bson.M{
			"status":     outboxcore.StatusPublishing,
			"updated_at": bson.M{"$lte": staleBefore},
		}, bson.D{{Key: "updated_at", Value: 1}, {Key: "created_at", Value: 1}}, now)
		if err != nil {
			return nil, err
		}
		if !found {
			break
		}
		claimed = append(claimed, item)
	}

	return claimed, nil
}

func (s *Store) claimOne(ctx context.Context, filter interface{}, sort bson.D, now time.Time) (outboxport.PendingEvent, bool, error) {
	update := bson.M{
		"$set": bson.M{
			"status":     outboxcore.StatusPublishing,
			"updated_at": now,
		},
	}
	opts := options.FindOneAndUpdate().
		SetSort(sort).
		SetReturnDocument(options.After)

	var po OutboxPO
	if err := s.coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(&po); err != nil {
		if err == mongo.ErrNoDocuments {
			return outboxport.PendingEvent{}, false, nil
		}
		return outboxport.PendingEvent{}, false, err
	}

	pending, err := outboxcore.DecodePendingEvent(po.EventID, po.PayloadJSON)
	if err != nil {
		transition := outboxcore.NewDecodeFailureTransition(err, time.Now())
		_ = s.MarkEventFailed(ctx, po.EventID, transition.LastError, transition.NextAttemptAt)
		return outboxport.PendingEvent{}, false, nil
	}

	return pending, true, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Store) MarkEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	transition := outboxcore.NewPublishedTransition(publishedAt)
	result, err := s.coll.UpdateOne(ctx, bson.M{"event_id": eventID}, bson.M{
		"$set": bson.M{
			"status":       transition.Status,
			"published_at": transition.PublishedAt,
			"updated_at":   transition.UpdatedAt,
		},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("outbox event %q not found", eventID)
	}
	return nil
}

func (s *Store) MarkEventFailed(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error {
	transition := outboxcore.NewFailedTransition(lastError, nextAttemptAt, time.Now())
	result, err := s.coll.UpdateOne(ctx, bson.M{"event_id": eventID}, bson.M{
		"$set": bson.M{
			"status":          transition.Status,
			"last_error":      transition.LastError,
			"next_attempt_at": transition.NextAttemptAt,
			"updated_at":      transition.UpdatedAt,
		},
		"$inc": bson.M{
			"attempt_count": transition.AttemptIncrement,
		},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("outbox event %q not found", eventID)
	}
	return nil
}

func (s *Store) OutboxStatusSnapshot(ctx context.Context, now time.Time) (outboxport.StatusSnapshot, error) {
	if s == nil || s.coll == nil {
		return outboxcore.BuildStatusSnapshot("mongo-domain-events", now, nil), nil
	}

	statuses := outboxcore.UnfinishedStatuses()
	observations := make([]outboxcore.StatusObservation, 0, len(statuses))
	for _, status := range statuses {
		count, err := s.coll.CountDocuments(ctx, bson.M{"status": status})
		if err != nil {
			return outboxport.StatusSnapshot{}, err
		}
		var oldestCreatedAt *time.Time
		if count > 0 {
			var oldest struct {
				CreatedAt time.Time `bson:"created_at"`
			}
			err := s.coll.FindOne(
				ctx,
				bson.M{"status": status},
				options.FindOne().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetProjection(bson.M{"created_at": 1}),
			).Decode(&oldest)
			if err != nil {
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
	return outboxcore.BuildStatusSnapshot("mongo-domain-events", now, observations), nil
}
