package aiexplanation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RetentionPolicy is the infrastructure projection of one explicitly approved
// lifecycle policy. A completely empty policy is allowed while the feature is
// disabled; partially configured policies are rejected.
type RetentionPolicy struct {
	Version                    string
	ParticipantRecordRetention time.Duration
	PromptEvaluationRetention  time.Duration
	CapacityLedgerRetention    time.Duration
}

func (p RetentionPolicy) Validate() error {
	version := strings.TrimSpace(p.Version)
	if version == "" && p.ParticipantRecordRetention == 0 && p.PromptEvaluationRetention == 0 && p.CapacityLedgerRetention == 0 {
		return nil
	}
	if version == "" || p.ParticipantRecordRetention <= 0 || p.PromptEvaluationRetention <= 0 || p.CapacityLedgerRetention <= 0 {
		return fmt.Errorf("AI explanation retention policy version and positive durations are required together")
	}
	return nil
}

func (p RetentionPolicy) Enabled() bool { return strings.TrimSpace(p.Version) != "" }

func expiresAfter(at time.Time, retention time.Duration) (*time.Time, error) {
	if at.IsZero() || retention <= 0 {
		return nil, fmt.Errorf("AI explanation terminal time and positive retention are required")
	}
	expiresAt := at.UTC().Add(retention)
	if !expiresAt.After(at) {
		return nil, fmt.Errorf("AI explanation retention expiration overflows terminal time")
	}
	return &expiresAt, nil
}

func capacityLedgerExpiresAt(budgetDay time.Time, retention time.Duration) (*time.Time, error) {
	if budgetDay.IsZero() || !budgetDay.Equal(time.Date(budgetDay.Year(), budgetDay.Month(), budgetDay.Day(), 0, 0, 0, 0, time.UTC)) {
		return nil, fmt.Errorf("AI explanation capacity budget day must be UTC midnight")
	}
	return expiresAfter(budgetDay.Add(24*time.Hour), retention)
}

func ttlIndex() mongo.IndexModel {
	return mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetName("ttl_ai_explanation_expires_at").SetExpireAfterSeconds(0),
	}
}

func rejectMissingTerminalExpiration(ctx context.Context, collection *mongo.Collection, terminalFilter bson.M) error {
	if collection == nil || len(terminalFilter) == 0 {
		return fmt.Errorf("AI explanation retention collection and terminal filter are required")
	}
	filter := bson.M{"$and": bson.A{
		terminalFilter,
		bson.M{"$or": bson.A{
			bson.M{"expires_at": bson.M{"$exists": false}},
			bson.M{"expires_at": bson.M{"$not": bson.M{"$type": "date"}}},
			bson.M{"retention_policy_version": bson.M{"$exists": false}},
			bson.M{"retention_policy_version": bson.M{"$not": bson.M{"$type": "string"}}},
			bson.M{"retention_policy_version": ""},
		}},
	}}
	var identity struct {
		DomainID any `bson:"domain_id"`
	}
	err := collection.FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{"domain_id": 1})).Decode(&identity)
	if err == mongo.ErrNoDocuments {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("terminal document %v has no valid expires_at and retention_policy_version; offline lifecycle backfill is required", identity.DomainID)
}
