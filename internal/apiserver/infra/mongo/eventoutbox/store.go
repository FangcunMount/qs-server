package eventoutbox

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrActiveSessionTransactionRequired = stderrors.New("active mongo session transaction required")

const mongoOutboxIndexCreationTimeout = 30 * time.Second

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
	ClaimToken    string    `bson:"claim_token,omitempty"`
}

func (OutboxPO) CollectionName() string {
	return "domain_event_outbox"
}

type Store struct {
	coll               *mongo.Collection
	limiter            backpressure.Acquirer
	publishingStaleFor time.Duration
	topicResolver      eventcatalog.TopicResolver
	priorityTiers      [][]string
}

func NewStore(db *mongo.Database) (*Store, error) {
	return NewStoreWithTopicResolver(db, eventcatalog.NewCatalog(nil))
}

type StoreOption func(*Store)

func WithPriorityEventTypes(eventTypes []string) StoreOption {
	return func(s *Store) {
		if len(eventTypes) == 0 {
			return
		}
		s.priorityTiers = [][]string{normalizePriorityEventTypes(eventTypes), nil}
	}
}

func WithPriorityTiers(tiers [][]string) StoreOption {
	return func(s *Store) {
		if len(tiers) == 0 {
			return
		}
		s.priorityTiers = tiers
	}
}

func NewStoreWithTopicResolver(db *mongo.Database, resolver eventcatalog.TopicResolver, opts ...StoreOption) (*Store, error) {
	if resolver == nil {
		resolver = eventcatalog.NewCatalog(nil)
	}
	store := &Store{
		coll:               db.Collection((&OutboxPO{}).CollectionName()),
		publishingStaleFor: outboxcore.DefaultPublishingStaleFor,
		topicResolver:      resolver,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(store)
		}
	}
	if err := store.ensureIndexes(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) ensureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, mongoOutboxIndexCreationTimeout)
	defer cancel()

	if _, err := s.coll.Indexes().CreateMany(ctx, mongoOutboxIndexModels()); err != nil {
		return fmt.Errorf("create mongo outbox indexes: %w", err)
	}

	return nil
}

func mongoOutboxIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
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
				{Key: "status", Value: 1},
				{Key: "event_type", Value: 1},
				{Key: "next_attempt_at", Value: 1},
				{Key: "created_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_pending_status_event_type_next_created").
				SetPartialFilterExpression(bson.M{"status": outboxcore.StatusPending}),
		},
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "event_type", Value: 1},
				{Key: "created_at", Value: 1},
				{Key: "next_attempt_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_pending_status_event_type_created_next").
				SetPartialFilterExpression(bson.M{"status": outboxcore.StatusPending}),
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
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "created_at", Value: 1},
			},
			Options: options.Index().SetName("idx_status_created_at"),
		},
		{
			Keys: bson.D{
				{Key: "claim_token", Value: 1},
				{Key: "status", Value: 1},
			},
			Options: options.Index().
				SetName("idx_claim_token_status").
				SetPartialFilterExpression(bson.M{"status": outboxcore.StatusPublishing}),
		},
	}
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
		items, err := s.claimPendingBatch(ctx, pendingQuota, now)
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

type pendingClaimQuery struct {
	filter bson.M
	sort   bson.D
}

func pendingClaimQueries(now time.Time, tiers [][]string) []pendingClaimQuery {
	sortByCreatedAt := bson.D{{Key: "created_at", Value: 1}}
	base := bson.M{
		"status":          outboxcore.StatusPending,
		"next_attempt_at": bson.M{"$lte": now},
	}
	if len(tiers) == 0 {
		return []pendingClaimQuery{{filter: base, sort: sortByCreatedAt}}
	}

	queries := make([]pendingClaimQuery, 0, len(tiers))
	for _, tier := range tiers {
		if len(tier) == 0 {
			queries = append(queries, pendingClaimQuery{filter: cloneBSONMap(base), sort: sortByCreatedAt})
			continue
		}
		priorityFilter := cloneBSONMap(base)
		priorityFilter["event_type"] = bson.M{"$in": normalizePriorityEventTypes(tier)}
		queries = append(queries, pendingClaimQuery{filter: priorityFilter, sort: sortByCreatedAt})
	}
	return queries
}

func cloneBSONMap(src bson.M) bson.M {
	dst := make(bson.M, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func normalizePriorityEventTypes(eventTypes []string) []string {
	if len(eventTypes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(eventTypes))
	normalized := make([]string, 0, len(eventTypes))
	for _, eventType := range eventTypes {
		eventType = strings.TrimSpace(eventType)
		if eventType == "" {
			continue
		}
		if _, ok := seen[eventType]; ok {
			continue
		}
		seen[eventType] = struct{}{}
		normalized = append(normalized, eventType)
	}
	return normalized
}

func (s *Store) claimDueByNextAttempt(ctx context.Context, status string, limit int, now time.Time) ([]outboxport.PendingEvent, error) {
	return s.claimBatchByFilter(ctx, bson.M{
		"status":          status,
		"next_attempt_at": bson.M{"$lte": now},
	}, bson.D{{Key: "next_attempt_at", Value: 1}, {Key: "created_at", Value: 1}}, limit, now)
}

func (s *Store) claimStalePublishing(ctx context.Context, limit int, now, staleBefore time.Time) ([]outboxport.PendingEvent, error) {
	return s.claimBatchByFilter(ctx, bson.M{
		"status":     outboxcore.StatusPublishing,
		"updated_at": bson.M{"$lte": staleBefore},
	}, bson.D{{Key: "updated_at", Value: 1}, {Key: "created_at", Value: 1}}, limit, now)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Store) MarkEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	transition := outboxcore.NewPublishedTransition(publishedAt)
	result, err := s.coll.UpdateOne(ctx, bson.M{"event_id": eventID}, bson.M{
		"$set": bson.M{
			"status":       transition.Status,
			"published_at": transition.PublishedAt,
			"updated_at":   transition.UpdatedAt,
		},
		"$unset": bson.M{"claim_token": ""},
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
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

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
		"$unset": bson.M{"claim_token": ""},
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
	observations, err := s.loadStatusObservations(ctx, statuses)
	if err != nil {
		return outboxport.StatusSnapshot{}, err
	}
	return outboxcore.BuildStatusSnapshot("mongo-domain-events", now, observations), nil
}

type mongoStatusObservation struct {
	Status          string    `bson:"_id"`
	Count           int64     `bson:"n"`
	OldestCreatedAt time.Time `bson:"oldest_created_at"`
}

func (s *Store) loadStatusObservations(ctx context.Context, statuses []string) ([]outboxcore.StatusObservation, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	cur, err := s.coll.Aggregate(
		ctx,
		outboxStatusSnapshotPipeline(statuses),
		options.Aggregate().SetHint("idx_status_created_at"),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	observations := make([]outboxcore.StatusObservation, 0, len(statuses))
	for cur.Next(ctx) {
		var row mongoStatusObservation
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		oldestCreatedAt := row.OldestCreatedAt
		observations = append(observations, outboxcore.StatusObservation{
			Status:          row.Status,
			Count:           row.Count,
			OldestCreatedAt: &oldestCreatedAt,
		})
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return observations, nil
}

func outboxStatusSnapshotPipeline(statuses []string) mongo.Pipeline {
	return mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "status", Value: bson.D{{Key: "$in", Value: statuses}}}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$status"},
			{Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "oldest_created_at", Value: bson.D{{Key: "$min", Value: "$created_at"}}},
		}}},
	}
}
