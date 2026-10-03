// Package originaleffect checks host business authority for one missing effect.
// It never resets an Outbox, creates retry authority, or publishes a message.
package originaleffect

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	appoutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	reportinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/input"
	mongoreport "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	checkpoint "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	evaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	sqlreport "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/interpretation"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"gorm.io/gorm"
)

const (
	EvaluationRetry = "evaluation.retry.requested"
	ReportInitial   = "evaluation.outcome.committed"
)

var ErrUnsafeSource = errors.New("original authority unavailable, changed, not due or already accepted")

type Request struct {
	EventID             string
	AssessmentID, OrgID uint64
	AcceptedBefore      time.Time
}
type Plan struct {
	EventID           string `json:"event_id"`
	EventType         string `json:"event_type"`
	AssessmentID      uint64 `json:"assessment_id"`
	OrgID             uint64 `json:"org_id"`
	PreviousRunID     string `json:"previous_run_id"`
	ExpectedAttempt   int    `json:"expected_attempt,omitempty"`
	OutcomeID         string `json:"outcome_id,omitempty"`
	TemplateID        string `json:"template_id,omitempty"`
	TemplateVersion   string `json:"template_version,omitempty"`
	SourceFingerprint string `json:"source_fingerprint"`
	original          message.Message
}

func (p Plan) RecoveryEventID() string  { return p.EventID }
func (p Plan) Message() message.Message { return p.original }

type Reader struct {
	SQL   *gorm.DB
	Mongo *mongo.Database
}
type intentRow struct {
	Producer, MessageID, Destination, EventType, SchemaVersion, Scope, ContentType, OccurredAt string
	Payload, Fingerprint                                                                       []byte
	State                                                                                      string
	Version, AttemptCount                                                                      uint64
	NextAttemptAt                                                                              time.Time
	TransportConfirmedAt                                                                       *time.Time
}

// Capture uses a read-only SQL snapshot. Mongo absence is NOT a distributed
// snapshot: callers reserve the original journal, capture again and invoke the
// normal consumer whose original claim/Generation uniqueness resolves races.
func (r Reader) Capture(ctx context.Context, request Request, now time.Time) (Plan, error) {
	if r.SQL == nil || request.EventID == "" || len(request.EventID) > 128 || request.AssessmentID == 0 || request.AssessmentID > math.MaxInt64 || request.OrgID == 0 || request.OrgID > math.MaxInt64 || request.AcceptedBefore.IsZero() || request.AcceptedBefore.After(now) {
		return Plan{}, ErrUnsafeSource
	}
	var result Plan
	err := r.SQL.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row intentRow
		if err := tx.Table("rm_outbox").Where("producer=? AND message_id=? AND destination=?", "qs-server", request.EventID, "qs.evaluation.lifecycle").Take(&row).Error; err != nil {
			return err
		}
		original, env, err := validateIntent(row, request, now)
		if err != nil {
			return err
		}
		var a evaluation.AssessmentPO
		// Deliberately unscoped: deleted prior effects are still acceptance evidence.
		if err := tx.Unscoped().First(&a, request.AssessmentID).Error; err != nil {
			return err
		}
		if a.ID.Uint64() != request.AssessmentID || a.OrgID != int64(request.OrgID) || a.DeletedAt != nil || a.TesteeID == 0 {
			return ErrUnsafeSource
		}
		var runs []checkpoint.RuntimeCheckpointPO
		if err := tx.Unscoped().Where("scope=? AND assessment_id=?", "evaluation_run", request.AssessmentID).Order("attempt_no DESC, id DESC").Limit(1).Find(&runs).Error; err != nil {
			return err
		}
		if len(runs) != 1 || runs[0].DeletedAt != nil {
			return ErrUnsafeSource
		}
		latest := runs[0]
		plan := Plan{EventID: row.MessageID, EventType: row.EventType, AssessmentID: request.AssessmentID, OrgID: request.OrgID, PreviousRunID: latest.ResourceID, original: original}
		var outcome *evaluation.EvaluationOutcomePO
		switch row.EventType {
		case EvaluationRetry:
			var data payload.EvaluationRequestedData
			if err := json.Unmarshal(env.Data, &data); err != nil {
				return err
			}
			if !validRetry(a, latest, data, row.MessageID, now) {
				return ErrUnsafeSource
			}
			var effects []struct{ ID uint64 }
			if err := tx.Table("evaluation_outcome").Where("assessment_id=?", request.AssessmentID).Select("id").Limit(1).Find(&effects).Error; err != nil {
				return err
			}
			if len(effects) != 0 {
				return ErrUnsafeSource
			}
			plan.ExpectedAttempt = data.ExpectedAttempt
		case ReportInitial:
			if r.Mongo == nil {
				return ErrUnsafeSource
			}
			var data payload.EvaluationOutcomeCommittedData
			if err := json.Unmarshal(env.Data, &data); err != nil {
				return err
			}
			id, err := strconv.ParseUint(data.OutcomeID, 10, 64)
			if err != nil || id == 0 || id > math.MaxInt64 {
				return ErrUnsafeSource
			}
			var stored evaluation.EvaluationOutcomePO
			if err := tx.Table("evaluation_outcome").Where("id=?", id).Take(&stored).Error; err != nil {
				return err
			}
			if !validInitial(a, latest, stored, data, request) {
				return ErrUnsafeSource
			}
			// Reuse the normal immutable Outcome decoder and report-input adapter.
			// Do not synthesize missing input from a current model/template catalog.
			record, err := evaluation.NewOutcomeRepository(tx).FindByID(ctx, meta.FromUint64(id))
			if err != nil || record == nil {
				return ErrUnsafeSource
			}
			frozen, err := reportinput.FromOutcomeRecord(appoutcome.FactRecord(record))
			if err != nil {
				return ErrUnsafeSource
			}
			var rejected []struct{ ID uint64 }
			if err := tx.Model(&sqlreport.AdmissionFailurePO{}).Select("id").Where("outcome_id=? OR event_id=?", id, row.MessageID).Limit(1).Find(&rejected).Error; err != nil {
				return err
			}
			if len(rejected) != 0 {
				return ErrUnsafeSource
			}
			// Primary read, no deleted filter: any Generation or artifact means a
			// previous acceptance. No template substitution or existing Run recovery.
			for _, collection := range []string{(mongoreport.ReportGenerationPO{}).CollectionName(), (mongoreport.InterpretReportPO{}).CollectionName()} {
				var exists bson.Raw
				err := r.Mongo.Collection(collection, options.Collection().SetReadPreference(readpref.Primary())).FindOne(ctx, bson.M{"outcome_id": id}, options.FindOne().SetProjection(bson.M{"_id": 1}).SetMaxTime(5*time.Second)).Decode(&exists)
				if err == nil {
					return ErrUnsafeSource
				}
				if !errors.Is(err, mongo.ErrNoDocuments) {
					return err
				}
			}
			outcome = &stored
			plan.OutcomeID = data.OutcomeID
			plan.TemplateID = frozen.Report.TemplateID
			plan.TemplateVersion = frozen.Report.TemplateVersion.String()
		default:
			return ErrUnsafeSource
		}
		// Hash the actual frozen authority and confirmation, never print its body.
		encoded, err := json.Marshal(struct {
			Intent     intentRow
			Assessment evaluation.AssessmentPO
			Run        checkpoint.RuntimeCheckpointPO
			Outcome    *evaluation.EvaluationOutcomePO
		}{row, a, latest, outcome})
		if err != nil {
			return err
		}
		hash := sha256.Sum256(encoded)
		plan.SourceFingerprint = hex.EncodeToString(hash[:])
		result = plan
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, err
}
func validateIntent(row intentRow, request Request, now time.Time) (message.Message, *domain.Envelope, error) {
	if row.Producer != "qs-server" || row.MessageID != request.EventID || row.Destination != "qs.evaluation.lifecycle" || row.Scope != "org:"+strconv.FormatUint(request.OrgID, 10) || row.State != "published" || row.SchemaVersion != "v1" || row.ContentType != "application/json" || row.Version == 0 || row.AttemptCount == 0 || row.TransportConfirmedAt == nil || row.TransportConfirmedAt.IsZero() || row.TransportConfirmedAt.After(now) || row.NextAttemptAt.After(now) {
		return message.Message{}, nil, ErrUnsafeSource
	}
	original, err := message.New(message.Input{Producer: row.Producer, ID: row.MessageID, Destination: row.Destination, EventType: row.EventType, SchemaVersion: row.SchemaVersion, Scope: row.Scope, ContentType: row.ContentType, OccurredAt: row.OccurredAt, Payload: row.Payload})
	if err != nil {
		return message.Message{}, nil, err
	}
	hash := original.Fingerprint()
	if !bytes.Equal(hash[:], row.Fingerprint) {
		return message.Message{}, nil, ErrUnsafeSource
	}
	wire, recognized, err := legacy.Decode(row.Payload)
	if err != nil || !recognized || wire.UUID != row.MessageID {
		return message.Message{}, nil, ErrUnsafeSource
	}
	env, err := domain.DecodeEnvelope(wire.Payload)
	if err != nil {
		return message.Message{}, nil, err
	}
	at, err := time.Parse(time.RFC3339Nano, row.OccurredAt)
	if err != nil || env.ID != row.MessageID || env.EventType != row.EventType || env.AggregateType != "Evaluation" || env.AggregateID != strconv.FormatUint(request.AssessmentID, 10) || !env.OccurredAt.Equal(at) || at.After(request.AcceptedBefore) || wire.Metadata["event_type"] != row.EventType || wire.Metadata["aggregate_type"] != "Evaluation" || wire.Metadata["aggregate_id"] != env.AggregateID || wire.Metadata["source"] == "" || wire.Metadata["occurred_at"] != env.OccurredAt.Format(domain.OccurredAtLayout) {
		return message.Message{}, nil, ErrUnsafeSource
	}
	return original, env, nil
}
func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func validRetry(a evaluation.AssessmentPO, run checkpoint.RuntimeCheckpointPO, d payload.EvaluationRequestedData, eventID string, now time.Time) bool {
	origin := d.AttemptOrigin
	originValid := (origin == "automatic" && d.ActionRequestID == "") || ((origin == "manual" || origin == "force") && d.ActionRequestID != "")
	return a.Status == "failed" && run.Status == "failed" && run.FinishedAt != nil && run.LeaseExpiresAt == nil && (run.Retryable || origin == "force") && run.ResourceID != "" && run.AttemptNo > 0 && uint64(run.AttemptNo) < math.MaxInt32 && d.ExpectedAttempt == int(run.AttemptNo) && d.Mode == "next_attempt" && originValid && value(run.RetryDisposition) == "automatic" && value(run.RetryEventID) == eventID && value(run.ActionRequestID) == d.ActionRequestID && run.NextAttemptAt != nil && !run.NextAttemptAt.After(now) && !d.RequestedAt.After(now) && sameSQLMillis(*run.NextAttemptAt, d.RequestedAt) && d.OrgID == a.OrgID && d.AssessmentID == int64(a.ID) && d.TesteeID == a.TesteeID && d.AnswerSheetID == strconv.FormatUint(a.AnswerSheetID, 10) && d.QuestionnaireCode == a.QuestionnaireCode && d.QuestionnaireVer == a.QuestionnaireVersion && d.ModelKind != "" && d.ModelCode != "" && d.ModelVersion != "" && d.ModelKind == value(a.EvaluationModelKind) && d.ModelAlgorithm == value(a.EvaluationModelAlgorithm) && d.ModelCode == value(a.EvaluationModelCode) && d.ModelVersion == value(a.EvaluationModelVersion)
}
func validInitial(a evaluation.AssessmentPO, run checkpoint.RuntimeCheckpointPO, o evaluation.EvaluationOutcomePO, d payload.EvaluationOutcomeCommittedData, r Request) bool {
	return a.Status == "evaluated" && run.Status == "succeeded" && run.ResourceID != "" && run.FinishedAt != nil && run.LeaseExpiresAt == nil && d.OrgID == a.OrgID && d.AssessmentID == int64(a.ID) && d.TesteeID == a.TesteeID && d.EvaluationRunID == run.ResourceID && o.AssessmentID == a.ID.Uint64() && o.OrgID == a.OrgID && o.TesteeID == a.TesteeID && o.EvaluationRunID == run.ResourceID && sameSQLMillis(o.EvaluatedAt, d.CommittedAt) && !o.EvaluatedAt.After(r.AcceptedBefore) && o.SchemaVersion > 0 && json.Valid([]byte(o.PayloadJSON)) && o.ReportInputJSON != nil && json.Valid([]byte(*o.ReportInputJSON)) && value(o.InputSnapshotRef) == value(run.InputSnapshotRef) && o.ModelKind != "" && o.ModelCode != "" && o.ModelVersion != "" && o.ModelKind == value(a.EvaluationModelKind) && value(o.ModelAlgorithm) == value(a.EvaluationModelAlgorithm) && o.ModelCode == value(a.EvaluationModelCode) && o.ModelVersion == value(a.EvaluationModelVersion)
}

// These business columns are DATETIME(3) in migrations 42 and 49. MySQL
// rounds fractional seconds by default; TIME_TRUNCATE_FRACTIONAL truncates.
// Accept only either actual storage representation, never a broad time range.
func sameSQLMillis(stored, original time.Time) bool {
	return stored.Equal(original.Round(time.Millisecond)) || stored.Equal(original.Truncate(time.Millisecond))
}
