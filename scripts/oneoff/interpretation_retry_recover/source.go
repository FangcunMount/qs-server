package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	interp "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const retryDestination = "qs.evaluation.lifecycle"
const retryType = "interpretation.retry.requested"

var errUnsafeSource = errors.New("original retry authority is missing, changed, uncertain or already claimed")

type originalIntent struct {
	Producer             string     `bson:"producer"`
	ID                   string     `bson:"message_id"`
	Destination          string     `bson:"destination"`
	EventType            string     `bson:"event_type"`
	SchemaVersion        string     `bson:"schema_version"`
	Scope                string     `bson:"scope"`
	ContentType          string     `bson:"content_type"`
	OccurredAt           string     `bson:"occurred_at"`
	Payload              []byte     `bson:"payload"`
	Fingerprint          []byte     `bson:"fingerprint"`
	State                string     `bson:"state"`
	TransportConfirmedAt *time.Time `bson:"transport_confirmed_at"`
}

type sourceSnapshot struct {
	Run        interp.InterpretationRunPO
	Generation interp.ReportGenerationPO
	Intent     originalIntent
}

type recoveryPlan struct {
	RunID             string `json:"run_id"`
	GenerationID      string `json:"generation_id"`
	EventID           string `json:"event_id"`
	OrgID             int64  `json:"org_id"`
	OriginalAttempt   int    `json:"original_attempt"`
	Origin            string `json:"origin"`
	ActionRequestID   string `json:"original_action_request_id,omitempty"`
	SourceFingerprint string `json:"source_fingerprint"`
	message           message.Message
}

// Capture uses a single primary snapshot and the existing host indexes. It
// reads one fixed Run and its original intent; it never resets published,
// creates authority, writes an attempt or calls a model.
func captureOriginal(ctx context.Context, db *mongo.Database, runID uint64, orgID int64, now time.Time) (recoveryPlan, error) {
	session, err := db.Client().StartSession()
	if err != nil {
		return recoveryPlan{}, err
	}
	defer session.EndSession(context.Background())
	if err := session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())); err != nil {
		return recoveryPlan{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = session.AbortTransaction(cleanup)
	}()
	sc := mongo.NewSessionContext(ctx, session)
	var source sourceSnapshot
	if err := db.Collection("interpretation_runs").FindOne(sc, bson.M{"domain_id": runID, "deleted_at": nil}, options.FindOne().SetHint("uk_interpretation_run_domain_id").SetMaxTime(5*time.Second)).Decode(&source.Run); err != nil {
		return recoveryPlan{}, err
	}
	if err := db.Collection("report_generations").FindOne(sc, bson.M{"domain_id": source.Run.GenerationID, "deleted_at": nil}, options.FindOne().SetHint("uk_generation_domain_id").SetMaxTime(5*time.Second)).Decode(&source.Generation); err != nil {
		return recoveryPlan{}, err
	}
	// A deleted successor still proves that the retry was accepted. Do not
	// restore its predecessor or turn a model-result uncertainty into a retry.
	successor := db.Collection("interpretation_runs").FindOne(sc, bson.M{"generation_id": source.Run.GenerationID, "attempt": bson.M{"$gt": source.Run.Attempt}}, options.FindOne().SetHint("uk_interpretation_run_generation_attempt").SetProjection(bson.M{"_id": 1}).SetMaxTime(5*time.Second))
	if !errors.Is(successor.Err(), mongo.ErrNoDocuments) {
		if successor.Err() != nil {
			return recoveryPlan{}, successor.Err()
		}
		return recoveryPlan{}, errUnsafeSource
	}
	id := bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: source.Run.RetryEventID}, {Key: "destination", Value: retryDestination}}
	if err := db.Collection("rm_outbox").FindOne(sc, bson.M{"_id": id}, options.FindOne().SetHint("_id_").SetMaxTime(5*time.Second)).Decode(&source.Intent); err != nil {
		return recoveryPlan{}, err
	}
	return validateOriginal(source, orgID, now)
}

func validateOriginal(source sourceSnapshot, orgID int64, now time.Time) (recoveryPlan, error) {
	r, g, row := source.Run, source.Generation, source.Intent
	if orgID <= 0 || r.DomainID.IsZero() || r.GenerationID == 0 || r.Attempt < 1 || r.DeletedAt != nil ||
		r.Status != "failed" || r.RetryDisposition != "automatic" || r.RetryPolicyVersion == "" || r.NextAttemptAt == nil || r.NextAttemptAt.After(now) || r.RetryEventID == "" || r.FinishedAt == nil || r.FinishedAt.After(now) ||
		g.DomainID.IsZero() || uint64(g.DomainID) != r.GenerationID || g.DeletedAt != nil || g.LatestRunID != uint64(r.DomainID) || g.Status != "failed" || g.ReportID != 0 || g.OutcomeID == 0 || g.TemplateVersion == "" || g.ReportType == "" || g.Version == 0 || g.TransactionSchemaVersion != 1 ||
		r.Failure == nil || r.Failure.Kind == "" || r.Failure.Code == "" || r.Failure.Kind == "timeout" || strings.Contains(strings.ToLower(r.Failure.Code), "unknown") || strings.Contains(strings.ToLower(r.Failure.Code), "timeout") ||
		row.State != "published" || row.TransportConfirmedAt == nil || row.TransportConfirmedAt.IsZero() || row.TransportConfirmedAt.After(now) || row.Producer != "qs-server" || row.ID != r.RetryEventID || row.Destination != retryDestination || row.EventType != retryType || row.SchemaVersion != "v1" || row.ContentType != "application/json" || row.Scope != fmt.Sprintf("org:%d", orgID) {
		return recoveryPlan{}, errUnsafeSource
	}
	original, err := message.New(message.Input{Producer: row.Producer, ID: row.ID, Destination: row.Destination, EventType: row.EventType, SchemaVersion: row.SchemaVersion, Scope: row.Scope, ContentType: row.ContentType, OccurredAt: row.OccurredAt, Payload: row.Payload})
	if err != nil {
		return recoveryPlan{}, errUnsafeSource
	}
	hash := original.Fingerprint()
	if !bytes.Equal(hash[:], row.Fingerprint) {
		return recoveryPlan{}, errUnsafeSource
	}
	wire, recognized, err := legacy.Decode(row.Payload)
	if err != nil || !recognized || wire.UUID != row.ID || wire.Metadata["event_type"] != retryType {
		return recoveryPlan{}, errUnsafeSource
	}
	env, err := domain.DecodeEnvelope(wire.Payload)
	if err != nil || env.ID != row.ID || env.EventType != retryType || env.AggregateType != "ReportGeneration" || env.AggregateID != g.DomainID.String() {
		return recoveryPlan{}, errUnsafeSource
	}
	var data eventoutcome.InterpretationRetryRequestedPayload
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return recoveryPlan{}, errUnsafeSource
	}
	if data.OrgID != orgID || data.GenerationID != g.DomainID.String() || data.RunID != r.DomainID.String() || data.OutcomeID != strconv.FormatUint(g.OutcomeID, 10) || data.ExpectedAttempt != r.Attempt || data.ActionRequestID != r.ActionRequestID || data.Mode != "next_attempt" || data.TesteeID == 0 || data.AssessmentID == "" || !data.RequestedAt.Truncate(time.Millisecond).Equal(r.NextAttemptAt.Truncate(time.Millisecond)) || !env.OccurredAt.Truncate(time.Millisecond).Equal(data.RequestedAt.Truncate(time.Millisecond)) {
		return recoveryPlan{}, errUnsafeSource
	}
	if (r.ActionRequestID == "" && data.AttemptOrigin != "automatic") || (r.ActionRequestID != "" && data.AttemptOrigin != "manual" && data.AttemptOrigin != "force") {
		return recoveryPlan{}, errUnsafeSource
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return recoveryPlan{}, err
	}
	sourceHash := sha256.Sum256(encoded)
	return recoveryPlan{RunID: r.DomainID.String(), GenerationID: g.DomainID.String(), EventID: row.ID, OrgID: orgID, OriginalAttempt: r.Attempt, Origin: data.AttemptOrigin, ActionRequestID: data.ActionRequestID, SourceFingerprint: hex.EncodeToString(sourceHash[:]), message: original}, nil
}
