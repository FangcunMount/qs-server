package attentionprojection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type projectionPO struct {
	EventID      string    `bson:"event_id"`
	ReportID     string    `bson:"report_id"`
	AssessmentID string    `bson:"assessment_id"`
	TesteeID     uint64    `bson:"testee_id"`
	RiskLevel    string    `bson:"risk_level"`
	MarkKeyFocus bool      `bson:"mark_key_focus"`
	Status       Status    `bson:"status"`
	Attempt      int       `bson:"attempt"`
	LastError    string    `bson:"last_error,omitempty"`
	CreatedAt    time.Time `bson:"created_at"`
	UpdatedAt    time.Time `bson:"updated_at"`
}

// MongoStore persists attention projection state in MongoDB.
type MongoStore struct {
	collection *mongo.Collection
}

func NewMongoStore(db *mongo.Database) (*MongoStore, error) {
	if db == nil {
		return nil, fmt.Errorf("mongo database is required")
	}
	store := &MongoStore{collection: db.Collection(CollectionName)}
	if _, err := store.collection.Indexes().CreateMany(context.Background(), mongoIndexModels()); err != nil {
		return nil, fmt.Errorf("create attention projection indexes: %w", err)
	}
	return store, nil
}

func mongoIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "event_id", Value: 1}},
			Options: options.Index().SetName("uk_attention_projection_event_id").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "status", Value: 1}, {Key: "updated_at", Value: 1}},
			Options: options.Index().SetName("idx_attention_projection_status_updated"),
		},
	}
}

func (s *MongoStore) EnsurePending(ctx context.Context, input PendingInput) (bool, error) {
	if input.EventID == "" {
		return false, fmt.Errorf("event_id is required")
	}
	now := time.Now().UTC()
	res, err := s.collection.UpdateOne(ctx,
		bson.M{"event_id": input.EventID},
		bson.M{
			"$setOnInsert": bson.M{
				"event_id":       input.EventID,
				"report_id":      input.ReportID,
				"assessment_id":  input.AssessmentID,
				"testee_id":      input.TesteeID,
				"risk_level":     input.RiskLevel,
				"mark_key_focus": input.MarkKeyFocus,
				"status":         StatusPending,
				"attempt":        0,
				"created_at":     now,
			},
			"$set": bson.M{
				"report_id":      input.ReportID,
				"assessment_id":  input.AssessmentID,
				"testee_id":      input.TesteeID,
				"risk_level":     input.RiskLevel,
				"mark_key_focus": input.MarkKeyFocus,
				"updated_at":     now,
			},
		},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return s.isAlreadySucceeded(ctx, input.EventID)
		}
		return false, fmt.Errorf("ensure attention projection pending: %w", err)
	}
	if res.UpsertedCount > 0 {
		return false, nil
	}
	return s.isAlreadySucceeded(ctx, input.EventID)
}

func (s *MongoStore) isAlreadySucceeded(ctx context.Context, eventID string) (bool, error) {
	rec, err := s.GetByEventID(ctx, eventID)
	if err != nil {
		return false, err
	}
	return rec.Status == StatusSucceeded, nil
}

func (s *MongoStore) MarkSucceeded(ctx context.Context, eventID string) error {
	now := time.Now().UTC()
	res, err := s.collection.UpdateOne(ctx,
		bson.M{"event_id": eventID},
		bson.M{"$set": bson.M{
			"status":     StatusSucceeded,
			"last_error": "",
			"updated_at": now,
		}},
	)
	if err != nil {
		return fmt.Errorf("mark attention projection succeeded: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("attention projection not found: %s", eventID)
	}
	return nil
}

func (s *MongoStore) RecordFailure(ctx context.Context, eventID string, errMsg string, maxAttempts int) (Status, error) {
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	rec, err := s.GetByEventID(ctx, eventID)
	if err != nil {
		return "", err
	}
	attempt := rec.Attempt + 1
	status := StatusFailed
	if attempt >= maxAttempts {
		status = StatusManualRequired
	}
	now := time.Now().UTC()
	res, err := s.collection.UpdateOne(ctx,
		bson.M{"event_id": eventID},
		bson.M{"$set": bson.M{
			"attempt":    attempt,
			"last_error": errMsg,
			"status":     status,
			"updated_at": now,
		}},
	)
	if err != nil {
		return "", fmt.Errorf("record attention projection failure: %w", err)
	}
	if res.MatchedCount == 0 {
		return "", fmt.Errorf("attention projection not found: %s", eventID)
	}
	return status, nil
}

func (s *MongoStore) GetByEventID(ctx context.Context, eventID string) (*Record, error) {
	var po projectionPO
	if err := s.collection.FindOne(ctx, bson.M{"event_id": eventID}).Decode(&po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("attention projection not found: %s", eventID)
		}
		return nil, fmt.Errorf("find attention projection: %w", err)
	}
	return poToRecord(&po), nil
}

func (s *MongoStore) ListRetryable(ctx context.Context, maxAttempts int, limit int) ([]Record, error) {
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	if limit <= 0 {
		limit = 100
	}
	cur, err := s.collection.Find(ctx,
		bson.M{
			"status":  bson.M{"$in": []Status{StatusPending, StatusFailed}},
			"attempt": bson.M{"$lt": maxAttempts},
		},
		options.Find().SetSort(bson.D{{Key: "updated_at", Value: 1}}).SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, fmt.Errorf("list retryable attention projections: %w", err)
	}
	defer cur.Close(ctx)

	items := make([]Record, 0)
	for cur.Next(ctx) {
		var po projectionPO
		if err := cur.Decode(&po); err != nil {
			return nil, fmt.Errorf("decode attention projection: %w", err)
		}
		items = append(items, *poToRecord(&po))
	}
	return items, cur.Err()
}

func poToRecord(po *projectionPO) *Record {
	if po == nil {
		return nil
	}
	return &Record{
		EventID:      po.EventID,
		ReportID:     po.ReportID,
		AssessmentID: po.AssessmentID,
		TesteeID:     po.TesteeID,
		RiskLevel:    po.RiskLevel,
		MarkKeyFocus: po.MarkKeyFocus,
		Status:       po.Status,
		Attempt:      po.Attempt,
		LastError:    po.LastError,
		CreatedAt:    po.CreatedAt,
		UpdatedAt:    po.UpdatedAt,
	}
}

var _ Store = (*MongoStore)(nil)
