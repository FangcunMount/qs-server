package eventoutbox

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/outboxcodec"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/pkg/event"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	statusPending    = "pending"
	statusPublishing = "publishing"
	statusPublished  = "published"
	statusFailed     = "failed"

	defaultPublishingStaleFor = 1 * time.Minute
)

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
	topicResolver      eventconfig.TopicResolver
}

func NewStore(db *mongo.Database) (*Store, error) {
	return NewStoreWithTopicResolver(db, eventconfig.Global())
}

func NewStoreWithTopicResolver(db *mongo.Database, resolver eventconfig.TopicResolver) (*Store, error) {
	if resolver == nil {
		resolver = eventconfig.Global()
	}
	store := &Store{
		coll:               db.Collection((&OutboxPO{}).CollectionName()),
		publishingStaleFor: defaultPublishingStaleFor,
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
				SetPartialFilterExpression(bson.M{"status": statusPending}),
		},
		{
			Keys: bson.D{
				{Key: "next_attempt_at", Value: 1},
				{Key: "created_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_failed_next_attempt_at_created_at").
				SetPartialFilterExpression(bson.M{"status": statusFailed}),
		},
		{
			Keys: bson.D{
				{Key: "updated_at", Value: 1},
				{Key: "created_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_publishing_updated_at_created_at").
				SetPartialFilterExpression(bson.M{"status": statusPublishing}),
		},
	}); err != nil {
		return fmt.Errorf("create mongo outbox indexes: %w", err)
	}

	return nil
}

func (s *Store) StageEventsTx(ctx mongo.SessionContext, events []event.DomainEvent) error {
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
	if len(events) == 0 {
		return nil, nil
	}

	now := time.Now()
	docs := make([]*OutboxPO, 0, len(events))
	for _, evt := range events {
		topicName, ok := s.topicResolver.GetTopicForEvent(evt.EventType())
		if !ok {
			return nil, fmt.Errorf("event %q not found in event config", evt.EventType())
		}
		payload, err := outboxcodec.Encode(evt)
		if err != nil {
			return nil, err
		}

		docs = append(docs, &OutboxPO{
			EventID:       evt.EventID(),
			EventType:     evt.EventType(),
			AggregateType: evt.AggregateType(),
			AggregateID:   evt.AggregateID(),
			TopicName:     topicName,
			PayloadJSON:   payload,
			Status:        statusPending,
			AttemptCount:  0,
			NextAttemptAt: now,
			CreatedAt:     now,
			UpdatedAt:     now,
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
		items, err := s.claimDueByNextAttempt(ctx, statusFailed, failedQuota, now)
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
		items, err := s.claimDueByNextAttempt(ctx, statusFailed, remaining, now)
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
			"status":          statusPending,
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
			"status":     statusPublishing,
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
			"status":     statusPublishing,
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

	evt, err := outboxcodec.Decode(po.PayloadJSON)
	if err != nil {
		_ = s.MarkEventFailed(ctx, po.EventID, fmt.Sprintf("decode outbox payload: %v", err), time.Now().Add(10*time.Second))
		return outboxport.PendingEvent{}, false, nil
	}

	return outboxport.PendingEvent{
		EventID: po.EventID,
		Event:   evt,
	}, true, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Store) MarkEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	_, err := s.coll.UpdateOne(ctx, bson.M{"event_id": eventID}, bson.M{
		"$set": bson.M{
			"status":       statusPublished,
			"published_at": publishedAt,
			"updated_at":   publishedAt,
		},
	})
	return err
}

func (s *Store) MarkEventFailed(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error {
	_, err := s.coll.UpdateOne(ctx, bson.M{"event_id": eventID}, bson.M{
		"$set": bson.M{
			"status":          statusFailed,
			"last_error":      lastError,
			"next_attempt_at": nextAttemptAt,
			"updated_at":      time.Now(),
		},
		"$inc": bson.M{
			"attempt_count": 1,
		},
	})
	return err
}
