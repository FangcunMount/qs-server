package attentionprojection

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const reportArtifactCollection = "interpret_report_artifacts"

type MongoFactSource struct {
	collection *mongo.Collection
}

func NewMongoFactSource(db *mongo.Database) (*MongoFactSource, error) {
	if db == nil {
		return nil, fmt.Errorf("mongo database is required")
	}
	return &MongoFactSource{collection: db.Collection(reportArtifactCollection)}, nil
}

type factCursor struct {
	GeneratedAt time.Time `json:"generated_at"`
	ReportID    uint64    `json:"report_id"`
}

type reportFactPO struct {
	ReportID     uint64    `bson:"domain_id"`
	AssessmentID uint64    `bson:"assessment_id"`
	TesteeID     uint64    `bson:"testee_id"`
	RiskLevel    string    `bson:"risk_level"`
	GeneratedAt  time.Time `bson:"generated_at"`
}

func (s *MongoFactSource) ListReportFacts(ctx context.Context, from time.Time, cursor string, limit int) ([]ReportFact, string, error) {
	if s == nil || s.collection == nil {
		return nil, "", fmt.Errorf("attention report fact source is not configured")
	}
	if from.IsZero() {
		return nil, "", fmt.Errorf("attention projection reconcile_from is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	after, err := decodeFactCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	query := bson.M{
		"deleted_at":   nil,
		"generated_at": bson.M{"$gte": from.UTC()},
		"risk_level":   bson.M{"$in": bson.A{"high", "severe"}},
	}
	if !after.GeneratedAt.IsZero() {
		query["$or"] = bson.A{
			bson.M{"generated_at": bson.M{"$gt": after.GeneratedAt}},
			bson.M{"generated_at": after.GeneratedAt, "domain_id": bson.M{"$gt": after.ReportID}},
		}
	}
	cur, err := s.collection.Find(ctx, query, options.Find().
		SetProjection(bson.M{"domain_id": 1, "assessment_id": 1, "testee_id": 1, "risk_level": 1, "generated_at": 1}).
		SetSort(bson.D{{Key: "generated_at", Value: 1}, {Key: "domain_id", Value: 1}}).
		SetLimit(int64(limit)))
	if err != nil {
		return nil, "", fmt.Errorf("list attention report facts: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	items := make([]ReportFact, 0, limit)
	var last factCursor
	for cur.Next(ctx) {
		var po reportFactPO
		if err := cur.Decode(&po); err != nil {
			return nil, "", fmt.Errorf("decode attention report fact: %w", err)
		}
		items = append(items, ReportFact{
			ReportID: strconv.FormatUint(po.ReportID, 10), AssessmentID: strconv.FormatUint(po.AssessmentID, 10),
			TesteeID: po.TesteeID, RiskLevel: po.RiskLevel, MarkKeyFocus: true, GeneratedAt: po.GeneratedAt,
		})
		last = factCursor{GeneratedAt: po.GeneratedAt, ReportID: po.ReportID}
	}
	if err := cur.Err(); err != nil {
		return nil, "", err
	}
	if len(items) < limit {
		return items, "", nil
	}
	next, err := encodeFactCursor(last)
	if err != nil {
		return nil, "", err
	}
	return items, next, nil
}

func encodeFactCursor(cursor factCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode attention fact cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeFactCursor(value string) (factCursor, error) {
	if value == "" {
		return factCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return factCursor{}, fmt.Errorf("invalid attention fact cursor")
	}
	var cursor factCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.GeneratedAt.IsZero() || cursor.ReportID == 0 {
		return factCursor{}, fmt.Errorf("invalid attention fact cursor")
	}
	return cursor, nil
}

var _ FactSource = (*MongoFactSource)(nil)
