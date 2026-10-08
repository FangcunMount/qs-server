package mongoconsistency

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	appaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	answersheetdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	evaluationfact "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/reliable-messaging/message"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type standardEventRow struct {
	ID            bson.Raw `bson:"_id"`
	Producer      string   `bson:"producer"`
	MessageID     string   `bson:"message_id"`
	Destination   string   `bson:"destination"`
	EventType     string   `bson:"event_type"`
	SchemaVersion string   `bson:"schema_version"`
	Scope         string   `bson:"scope"`
	ContentType   string   `bson:"content_type"`
	OccurredAt    string   `bson:"occurred_at"`
	Payload       []byte   `bson:"payload"`
	Fingerprint   []byte   `bson:"fingerprint"`
}

func (r standardEventRow) input() message.Input {
	return message.Input{Producer: r.Producer, ID: r.MessageID, Destination: r.Destination, EventType: r.EventType, SchemaVersion: r.SchemaVersion, Scope: r.Scope, ContentType: r.ContentType, OccurredAt: r.OccurredAt, Payload: r.Payload}
}
func identity(ref evidence.StandardReference) bson.D {
	return bson.D{{Key: "producer", Value: ref.Producer}, {Key: "message_id", Value: ref.EventID}, {Key: "destination", Value: ref.Destination}}
}

// Tokens retain the SDK's BSON field order and string types. They are never
// converted to ObjectIDs, numeric aggregate IDs, maps, or Extended JSON.
func decodeOutboxToken(raw []byte) (bson.D, error) {
	if len(raw) == 0 || len(raw) > 1024 {
		return nil, driftf("invalid standard outbox BSON cursor size")
	}
	if err := bson.Raw(raw).Validate(); err != nil {
		return nil, driftf("invalid standard outbox BSON cursor: %w", err)
	}
	var token bson.D
	if err := bson.Unmarshal(raw, &token); err != nil {
		return nil, err
	}
	keys := []string{"producer", "message_id", "destination"}
	if len(token) != len(keys) {
		return nil, driftf("invalid standard outbox BSON identity")
	}
	for i, key := range keys {
		v, ok := token[i].Value.(string)
		if token[i].Key != key || !ok || v == "" {
			return nil, driftf("invalid standard outbox BSON identity")
		}
	}
	in := message.Input{Producer: token[0].Value.(string), ID: token[1].Value.(string), Destination: token[2].Value.(string), EventType: "cursor.validation", SchemaVersion: "v1", Scope: "org:1", ContentType: "application/json", OccurredAt: "2026-01-01T00:00:00Z", Payload: []byte("{}")}
	if _, err := message.New(in); err != nil {
		return nil, driftf("invalid standard outbox cursor fields: %w", err)
	}
	return token, nil
}
func (s *Scanner) OutboxUpperBound(ctx context.Context, maxTime time.Duration) ([]byte, error) {
	if s == nil || s.db == nil || maxTime <= 0 {
		return nil, driftf("mongo consistency scanner is not configured")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, maxTime)
	defer cancel()
	var row struct {
		ID bson.Raw `bson:"_id"`
	}
	err = s.db.Collection("rm_outbox").FindOne(ctx, bson.D{}, options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}}).SetHint("_id_").SetProjection(bson.M{"_id": 1}).SetMaxTime(maxTime)).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := decodeOutboxToken(row.ID); err != nil {
		return nil, err
	}
	return append([]byte(nil), row.ID...), nil
}
func answerSheetAuditFilter() bson.M {
	return bson.M{"deleted_at": nil, "$or": bson.A{bson.M{"durable_acceptance.schema_version": 1}, bson.M{"durable_acceptance.event_evidence": bson.M{"$exists": true}}, bson.M{"legacy_submission_evidence": bson.M{"$exists": true}}}}
}
func generatedAuditFilter() bson.M {
	return bson.M{"deleted_at": nil, "status": "generated", "$or": bson.A{bson.M{"transaction_schema_version": 1}, bson.M{"generated_event_evidence": bson.M{"$exists": true}}, bson.M{"historical_generated_evidence": bson.M{"$exists": true}}}}
}

func retryAuditFilter() bson.M {
	return bson.M{"deleted_at": nil, "$or": bson.A{bson.M{"retry_event_id": bson.M{"$type": "string", "$ne": ""}}, bson.M{"retry_event_evidence": bson.M{"$exists": true}}}}
}

var sheetAuditProjection = bson.M{"domain_id": 1, "org_id": 1, "testee_id": 1, "filler_id": 1, "filler_type": 1, "questionnaire_code": 1, "questionnaire_version": 1, "task_id": 1, "admission": 1, "attribution": 1, "filled_at": 1, "submit_meta.request_id": 1, "durable_acceptance": 1, "legacy_submission_evidence": 1}
var generationAuditProjection = bson.M{"domain_id": 1, "outcome_id": 1, "report_type": 1, "template_version": 1, "status": 1, "latest_run_id": 1, "report_id": 1, "version": 1, "transaction_schema_version": 1, "generated_event_id": 1, "generated_event_evidence": 1, "historical_generated_evidence": 1}
var runAuditProjection = bson.M{"domain_id": 1, "generation_id": 1, "attempt": 1, "status": 1, "retry_disposition": 1, "next_attempt_at": 1, "retry_event_id": 1, "retry_event_evidence": 1, "action_request_id": 1}
var artifactAuditProjection = bson.M{"domain_id": 1, "generation_id": 1, "outcome_id": 1, "interpretation_run_id": 1, "org_id": 1, "assessment_id": 1, "testee_id": 1, "report_type": 1, "template_version": 1, "builder_identity": 1, "content_schema_version": 1, "generated_at": 1, "model": 1, "primary_score": 1, "level": 1}

func (s *Scanner) standardRow(ctx context.Context, ref evidence.StandardReference) (standardEventRow, error) {
	var row standardEventRow
	err := s.db.Collection("rm_outbox").FindOne(ctx, bson.D{{Key: "_id", Value: identity(ref)}}, options.FindOne().SetHint("_id_")).Decode(&row)
	return row, err
}
func verifyRow(row standardEventRow, ref evidence.StandardReference) (*domainwire.Envelope, error) {
	token, err := decodeOutboxToken(row.ID)
	if err != nil {
		return nil, err
	}
	if token[0].Value != row.Producer || token[1].Value != row.MessageID || token[2].Value != row.Destination || len(row.Fingerprint) != 32 {
		return nil, driftf("standard row identity conflict")
	}
	envelope, err := standard.VerifyReference(row.input(), hex.EncodeToString(row.Fingerprint), ref)
	return envelope, conflict(err)
}

// Verify immutable storage identity and both wire layers before trusting a row's
// event type for filtering. A corrupt outer type must not hide a covered orphan.
func verifyStoredRow(row standardEventRow) (*domainwire.Envelope, error) {
	msg, err := message.New(row.input())
	if err != nil {
		return nil, conflict(err)
	}
	return verifyRow(row, standard.ReferenceFromMessage(msg))
}
func countEvidence(result *appaudit.BatchResult, proof *evidence.EventEvidenceV1) {
	if result.EvidenceClasses == nil {
		result.EvidenceClasses = map[string]int64{}
	}
	result.EvidenceClasses[string(proof.Class)]++
}

// Historical conclusions are counted separately from live standard evidence;
// they cannot recreate an original message after its source body is retired.
func (s *Scanner) checkEvidence(ctx context.Context, proof *evidence.EventEvidenceV1, eventID, eventType, aggregateType, aggregateID, binding string, typed func([]byte) (string, error)) error {
	if proof == nil {
		return driftf("original event evidence is missing")
	}
	if err := proof.Validate(); err != nil {
		return conflict(err)
	}
	if proof.EventID != eventID || proof.BusinessBindingSHA256 != binding {
		return driftf("business event binding conflict")
	}
	if proof.Class != evidence.StandardReferenceClass {
		return nil
	}
	if proof.Reference.EventType != eventType {
		return driftf("business event type conflict")
	}
	row, err := s.standardRow(ctx, *proof.Reference)
	if err != nil {
		return err
	}
	envelope, err := verifyRow(row, *proof.Reference)
	if err != nil {
		return err
	}
	if envelope.AggregateType != aggregateType || envelope.AggregateID != aggregateID {
		return driftf("business aggregate identity conflict")
	}
	actual, err := typed(envelope.Data)
	if err != nil {
		return conflict(err)
	}
	if actual != binding {
		return driftf("typed original business payload conflict")
	}
	return nil
}
func sheetPayload(row sheetmongo.AnswerSheetPO) (eventpayload.AnswerSheetSubmittedData, error) {
	if row.DurableAcceptance == nil || row.FillerID < 0 {
		return eventpayload.AnswerSheetSubmittedData{}, driftf("missing submission ownership")
	}
	requestID := row.DurableAcceptance.RequestID
	if row.SubmitMeta != nil && row.SubmitMeta.RequestID != "" {
		if requestID != "" && requestID != row.SubmitMeta.RequestID {
			return eventpayload.AnswerSheetSubmittedData{}, driftf("submission request identity conflict")
		}
		// A stored native idempotent trace is factual; no new client trace is used.
		requestID = row.SubmitMeta.RequestID
	}
	data := eventpayload.AnswerSheetSubmittedData{AnswerSheetID: row.DomainID.String(), OrgID: row.OrgID, TesteeID: row.TesteeID, FillerID: uint64(row.FillerID), FillerType: row.FillerType, QuestionnaireCode: row.QuestionnaireCode, QuestionnaireVersion: row.QuestionnaireVersion, TaskID: row.TaskID, RequestID: requestID, SubmittedAt: row.FilledAt}
	if v := row.Admission; v != nil {
		data.Admission = &eventpayload.AssessmentAdmission{Purpose: eventpayload.AdmissionPurpose(v.Purpose), QuestionnaireCode: v.QuestionnaireCode, QuestionnaireVersion: v.QuestionnaireVersion, ModelKind: v.ModelKind, ModelAlgorithm: v.ModelAlgorithm, ModelCode: v.ModelCode, ModelVersion: v.ModelVersion, ModelTitle: v.ModelTitle}
	}
	if v := row.Attribution; v != nil {
		data.Attribution = &eventpayload.AttributionSnapshot{OriginType: v.OriginType, OriginID: v.OriginID, ClinicianID: v.ClinicianID, EntryID: v.EntryID, PlanID: v.PlanID, EnrollmentID: v.EnrollmentID, TaskID: v.TaskID, CapturedAt: v.CapturedAt, Version: v.Version, Mode: v.Mode}
	}
	return data, nil
}
func (s *Scanner) checkSheet(ctx context.Context, row sheetmongo.AnswerSheetPO) error {
	data, err := sheetPayload(row)
	if err != nil {
		return err
	}
	binding, err := eventevidencebinding.AnswerSheet(data)
	if err != nil {
		return conflict(err)
	}
	return s.checkEvidence(ctx, row.DurableAcceptance.EventEvidence, row.DurableAcceptance.EventID, eventcatalog.AnswerSheetSubmitted, answersheetdomain.AggregateType, row.DomainID.String(), binding, func(raw []byte) (string, error) {
		var p eventpayload.AnswerSheetSubmittedData
		if err := json.Unmarshal(raw, &p); err != nil {
			return "", err
		}
		return eventevidencebinding.AnswerSheet(p)
	})
}
func (s *Scanner) scanAnswerSheetOutbox(ctx context.Context, request appaudit.BatchRequest) (appaudit.BatchResult, error) {
	var rows []sheetmongo.AnswerSheetPO
	if err := findAll(ctx, s.db.Collection("answersheets"), boundedFilter(answerSheetAuditFilter(), request), findOptions(request, sheetAuditProjection), &rows); err != nil {
		return appaudit.BatchResult{}, err
	}
	result := appaudit.BatchResult{Scanned: len(rows)}
	for _, row := range rows {
		result.NextID = row.DomainID.Uint64()
		if row.LegacySubmissionEvidence != nil {
			err := s.checkHistoricalSlot(ctx, "answersheets", row.DomainID.Uint64(), "legacy_submission_evidence", row.LegacySubmissionEvidence)
			if err == nil {
				err = checkLegacySubmission(row)
			}
			if err != nil {
				if !isEvidenceDrift(err) {
					return result, err
				}
				result.Findings = append(result.Findings, finding(appaudit.DriftAnswerSheetMissingOutbox, result.NextID))
			} else {
				for _, entry := range row.LegacySubmissionEvidence.Entries {
					countEvidence(&result, entry.Proof)
				}
			}
			// A separate historical slot never claims atomic durable acceptance.
			if row.DurableAcceptance == nil {
				continue
			}
		}
		if err := s.checkSheet(ctx, row); err != nil {
			if !isEvidenceDrift(err) {
				return result, err
			}
			result.Findings = append(result.Findings, finding(appaudit.DriftAnswerSheetMissingOutbox, result.NextID))
		} else {
			countEvidence(&result, row.DurableAcceptance.EventEvidence)
		}
	}
	result.Exhausted = batchDone(result.Scanned, result.NextID, request)
	return result, nil
}

// Infrastructure failures abort the batch and preserve its checkpoint. Missing
// rows or invalid identities are findings, never a swallowed network failure.
type evidenceConflict struct{ cause error }

func (e evidenceConflict) Error() string { return e.cause.Error() }
func (e evidenceConflict) Unwrap() error { return e.cause }
func conflict(err error) error {
	if err == nil {
		return nil
	}
	return evidenceConflict{cause: err}
}
func driftf(format string, args ...interface{}) error { return conflict(fmt.Errorf(format, args...)) }
func isEvidenceDrift(err error) bool {
	var conflict evidenceConflict
	return errors.As(err, &conflict) || errors.Is(err, mongo.ErrNoDocuments) || errors.Is(err, evaluationfact.ErrNotFound)
}
func (s *Scanner) outcome(ctx context.Context, id uint64) (*evaluationfact.Record, error) {
	if s.outcomes == nil {
		return nil, fmt.Errorf("mongo standard audit requires evaluation fact reader")
	}
	fact, err := s.outcomes.FindByID(ctx, meta.ID(id))
	if err != nil {
		return nil, err
	}
	if fact == nil || fact.ID().Uint64() != id {
		return nil, driftf("generation outcome is missing")
	}
	return fact, nil
}
func (s *Scanner) generatedPayload(ctx context.Context, g interpretmongo.ReportGenerationPO) (eventoutcome.ReportGeneratedPayload, error) {
	var a interpretmongo.InterpretReportPO
	var r interpretmongo.InterpretationRunPO
	if err := s.db.Collection("interpret_report_artifacts").FindOne(ctx, bson.M{"domain_id": g.ReportID, "deleted_at": nil}, options.FindOne().SetProjection(artifactAuditProjection)).Decode(&a); err != nil {
		return eventoutcome.ReportGeneratedPayload{}, err
	}
	if err := s.db.Collection("interpretation_runs").FindOne(ctx, bson.M{"domain_id": g.LatestRunID, "deleted_at": nil}, options.FindOne().SetProjection(runAuditProjection)).Decode(&r); err != nil {
		return eventoutcome.ReportGeneratedPayload{}, err
	}
	fact, err := s.outcome(ctx, g.OutcomeID)
	if err != nil {
		return eventoutcome.ReportGeneratedPayload{}, err
	}
	if g.Status != "generated" || g.DomainID.IsZero() || a.GenerationID != g.DomainID.Uint64() || a.OutcomeID != g.OutcomeID || a.InterpretationRunID != g.LatestRunID || r.GenerationID != g.DomainID.Uint64() || r.Status != "succeeded" || a.OrgID != fact.OrgID() || a.AssessmentID != fact.AssessmentID().Uint64() || a.TesteeID != fact.TesteeID() || a.ReportType != g.ReportType || a.TemplateVersion != g.TemplateVersion || a.Model == nil {
		return eventoutcome.ReportGeneratedPayload{}, driftf("generated business graph conflict")
	}
	p := eventoutcome.ReportGeneratedPayload{OrgID: a.OrgID, GenerationID: g.DomainID.String(), RunID: strconv.FormatUint(a.InterpretationRunID, 10), ReportID: a.DomainID.String(), AssessmentID: strconv.FormatUint(a.AssessmentID, 10), OutcomeID: strconv.FormatUint(a.OutcomeID, 10), TesteeID: a.TesteeID, Attempt: uint(r.Attempt), ReportType: a.ReportType, TemplateVersion: a.TemplateVersion, BuilderIdentity: a.BuilderIdentity, ContentSchemaVersion: a.ContentSchemaVersion, GeneratedAt: a.GeneratedAt, Model: eventoutcome.ModelIdentity{Kind: a.Model.Kind, Algorithm: a.Model.Algorithm, Code: a.Model.Code, Version: a.Model.Version, Title: a.Model.Title}}
	if a.PrimaryScore != nil {
		v := a.PrimaryScore
		p.PrimaryScore = &eventoutcome.ScoreValue{Kind: v.Kind, Value: v.Value, Label: v.Label, Max: v.Max}
	}
	if a.Level != nil {
		v := a.Level
		p.Level = &eventoutcome.ResultLevel{Code: v.Code, Label: v.Label, Severity: v.Severity}
	}
	return p, nil
}
func (s *Scanner) checkGenerated(ctx context.Context, g interpretmongo.ReportGenerationPO) error {
	data, err := s.generatedPayload(ctx, g)
	if err != nil {
		return err
	}
	binding, err := eventevidencebinding.Generated(data)
	if err != nil {
		return conflict(err)
	}
	return s.checkEvidence(ctx, g.GeneratedEventEvidence, g.GeneratedEventID, eventcatalog.InterpretationReportGenerated, domaininterpretation.AggregateType, g.DomainID.String(), binding, func(raw []byte) (string, error) {
		var p eventoutcome.ReportGeneratedPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return "", err
		}
		return eventevidencebinding.Generated(p)
	})
}
func (s *Scanner) scanGeneratedTerminal(ctx context.Context, request appaudit.BatchRequest) (appaudit.BatchResult, error) {
	if s.outcomes == nil {
		return appaudit.BatchResult{}, driftf("mongo standard audit requires evaluation fact reader")
	}
	var rows []interpretmongo.ReportGenerationPO
	if err := findAll(ctx, s.db.Collection("report_generations"), boundedFilter(generatedAuditFilter(), request), findOptions(request, generationAuditProjection), &rows); err != nil {
		return appaudit.BatchResult{}, err
	}
	result := appaudit.BatchResult{Scanned: len(rows)}
	for _, g := range rows {
		result.NextID = g.DomainID.Uint64()
		var artifact struct {
			GenerationID uint64 `bson:"generation_id"`
		}
		err := s.db.Collection("interpret_report_artifacts").FindOne(ctx, bson.M{"domain_id": g.ReportID, "deleted_at": nil}, options.FindOne().SetProjection(bson.M{"generation_id": 1})).Decode(&artifact)
		if err != nil || artifact.GenerationID != g.DomainID.Uint64() {
			if err != nil && !isEvidenceDrift(err) {
				return result, err
			}
			result.Findings = append(result.Findings, finding(appaudit.DriftGeneratedMissingArtifact, result.NextID))
		}
		if g.HistoricalGeneratedEvidence != nil {
			if err := s.checkHistoricalGenerated(ctx, g); err != nil {
				if !isEvidenceDrift(err) {
					return result, err
				}
				result.Findings = append(result.Findings, finding(appaudit.DriftGeneratedMissingTerminalOutbox, result.NextID))
			} else {
				for _, entry := range g.HistoricalGeneratedEvidence.Entries {
					countEvidence(&result, entry.Proof)
				}
			}
		}
		if g.TransactionSchemaVersion != 1 && g.GeneratedEventEvidence == nil && g.HistoricalGeneratedEvidence != nil {
			continue
		}
		if err := s.checkGenerated(ctx, g); err != nil {
			if !isEvidenceDrift(err) {
				return result, err
			}
			result.Findings = append(result.Findings, finding(appaudit.DriftGeneratedMissingTerminalOutbox, result.NextID))
		} else {
			countEvidence(&result, g.GeneratedEventEvidence)
		}
	}
	result.Exhausted = batchDone(result.Scanned, result.NextID, request)
	return result, nil
}
func retryOrigin(row interpretmongo.InterpretationRunPO) (string, error) {
	prefix := fmt.Sprintf("interpret-retry:%d:%d:", row.GenerationID, row.Attempt)
	suffix := strings.TrimPrefix(row.RetryEventID, prefix)
	if suffix == row.RetryEventID {
		return "", driftf("retry identity prefix conflict")
	}
	if row.ActionRequestID != "" {
		end := ":" + row.ActionRequestID
		if !strings.HasSuffix(suffix, end) {
			return "", driftf("retry action identity conflict")
		}
		suffix = strings.TrimSuffix(suffix, end)
		if suffix != "manual" && suffix != "force" {
			return "", driftf("retry authorization origin conflict")
		}
	} else if suffix != "automatic" {
		return "", driftf("retry automatic origin conflict")
	}
	return suffix, nil
}
func (s *Scanner) retryPayload(ctx context.Context, r interpretmongo.InterpretationRunPO) (eventoutcome.InterpretationRetryRequestedPayload, error) {
	var g interpretmongo.ReportGenerationPO
	if err := s.db.Collection("report_generations").FindOne(ctx, bson.M{"domain_id": r.GenerationID, "deleted_at": nil}, options.FindOne().SetProjection(generationAuditProjection)).Decode(&g); err != nil {
		return eventoutcome.InterpretationRetryRequestedPayload{}, err
	}
	fact, err := s.outcome(ctx, g.OutcomeID)
	if err != nil {
		return eventoutcome.InterpretationRetryRequestedPayload{}, err
	}
	origin, err := retryOrigin(r)
	if err != nil {
		return eventoutcome.InterpretationRetryRequestedPayload{}, err
	}
	if r.Status != "failed" || r.Attempt < 1 || r.NextAttemptAt == nil || r.NextAttemptAt.IsZero() || r.RetryDisposition != "automatic" {
		return eventoutcome.InterpretationRetryRequestedPayload{}, driftf("retry business schedule conflict")
	}
	return eventoutcome.InterpretationRetryRequestedPayload{OrgID: fact.OrgID(), GenerationID: strconv.FormatUint(r.GenerationID, 10), RunID: r.DomainID.String(), AssessmentID: fact.AssessmentID().String(), OutcomeID: fact.ID().String(), TesteeID: fact.TesteeID(), ExpectedAttempt: r.Attempt, AttemptOrigin: origin, ActionRequestID: r.ActionRequestID, Mode: "next_attempt", RequestedAt: *r.NextAttemptAt}, nil
}
func (s *Scanner) checkRetry(ctx context.Context, r interpretmongo.InterpretationRunPO) error {
	data, err := s.retryPayload(ctx, r)
	if err != nil {
		return err
	}
	binding, err := eventevidencebinding.Retry(data)
	if err != nil {
		return conflict(err)
	}
	return s.checkEvidence(ctx, r.RetryEventEvidence, r.RetryEventID, eventcatalog.InterpretationRetryRequested, domaininterpretation.AggregateType, strconv.FormatUint(r.GenerationID, 10), binding, func(raw []byte) (string, error) {
		var p eventoutcome.InterpretationRetryRequestedPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return "", err
		}
		originalAt, err := time.Parse(time.RFC3339Nano, r.RetryEventEvidence.Reference.OccurredAt)
		if err != nil || !p.RequestedAt.Equal(originalAt) {
			return "", fmt.Errorf("retry original payload and event clock conflict")
		}
		return eventevidencebinding.Retry(p)
	})
}
func (s *Scanner) scanRetryOutbox(ctx context.Context, request appaudit.BatchRequest) (appaudit.BatchResult, error) {
	if s.outcomes == nil {
		return appaudit.BatchResult{}, driftf("mongo standard audit requires evaluation fact reader")
	}
	base := retryAuditFilter()
	var rows []interpretmongo.InterpretationRunPO
	if err := findAll(ctx, s.db.Collection("interpretation_runs"), boundedFilter(base, request), findOptions(request, runAuditProjection), &rows); err != nil {
		return appaudit.BatchResult{}, err
	}
	result := appaudit.BatchResult{Scanned: len(rows)}
	for _, r := range rows {
		result.NextID = r.DomainID.Uint64()
		if err := s.checkRetry(ctx, r); err != nil {
			if !isEvidenceDrift(err) {
				return result, err
			}
			result.Findings = append(result.Findings, finding(appaudit.DriftRetryMissingScheduledOutbox, result.NextID))
		} else {
			countEvidence(&result, r.RetryEventEvidence)
		}
	}
	result.Exhausted = batchDone(result.Scanned, result.NextID, request)
	return result, nil
}
func coveredEventType(kind string) bool {
	return kind == eventcatalog.AnswerSheetSubmitted || kind == eventcatalog.InterpretationReportGenerated || kind == eventcatalog.InterpretationRetryRequested
}
func (s *Scanner) scanOutboxAnswerSheet(ctx context.Context, request appaudit.BatchRequest) (appaudit.BatchResult, error) {
	if len(request.OutboxUpperBound) == 0 {
		if len(request.OutboxCursor) != 0 {
			return appaudit.BatchResult{}, driftf("outbox cursor without frozen upper bound")
		}
		return appaudit.BatchResult{Exhausted: true}, nil
	}
	upper, err := decodeOutboxToken(request.OutboxUpperBound)
	if err != nil {
		return appaudit.BatchResult{}, err
	}
	rangeQuery := bson.D{{Key: "$lte", Value: upper}}
	if len(request.OutboxCursor) > 0 {
		after, err := decodeOutboxToken(request.OutboxCursor)
		if err != nil {
			return appaudit.BatchResult{}, err
		}
		rangeQuery = append(rangeQuery, bson.E{Key: "$gt", Value: after})
	}
	var rows []standardEventRow
	if err := findAll(ctx, s.db.Collection("rm_outbox"), bson.D{{Key: "_id", Value: rangeQuery}}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetHint("_id_").SetLimit(int64(request.Limit)).SetMaxTime(request.MaxTime), &rows); err != nil {
		return appaudit.BatchResult{}, err
	}
	result := appaudit.BatchResult{Scanned: len(rows)}
	for _, row := range rows {
		if _, err := decodeOutboxToken(row.ID); err != nil {
			return result, err
		}
		result.NextOutboxCursor = append([]byte(nil), row.ID...)
		inner, err := verifyStoredRow(row)
		if err != nil {
			if !isEvidenceDrift(err) {
				return result, err
			}
			result.Findings = append(result.Findings, appaudit.Finding{Kind: appaudit.DriftOutboxBusinessMismatch, Severity: appaudit.SeverityHigh, SampleID: row.MessageID})
			continue
		}
		if !coveredEventType(inner.EventType) {
			continue
		}
		if err := s.checkReverse(ctx, row); err != nil {
			if !isEvidenceDrift(err) {
				return result, err
			}
			result.Findings = append(result.Findings, appaudit.Finding{Kind: appaudit.DriftOutboxBusinessMismatch, Severity: appaudit.SeverityHigh, SampleID: row.MessageID})
		}
	}
	result.Exhausted = result.Scanned < request.Limit || bytes.Equal(result.NextOutboxCursor, request.OutboxUpperBound)
	return result, nil
}
func (s *Scanner) checkReverse(ctx context.Context, row standardEventRow) error {
	outer, recognized, err := legacy.Decode(row.Payload)
	if err != nil || !recognized {
		return driftf("invalid reverse transport envelope")
	}
	inner, err := domainwire.DecodeEnvelope(outer.Payload)
	if err != nil {
		return conflict(err)
	}
	var proof *evidence.EventEvidenceV1
	switch row.EventType {
	case eventcatalog.AnswerSheetSubmitted:
		var p eventpayload.AnswerSheetSubmittedData
		if err := json.Unmarshal(inner.Data, &p); err != nil {
			return conflict(err)
		}
		id, err := strconv.ParseUint(p.AnswerSheetID, 10, 64)
		if err != nil || id == 0 {
			return driftf("invalid answersheet identity")
		}
		var sheet sheetmongo.AnswerSheetPO
		if err := s.db.Collection("answersheets").FindOne(ctx, bson.M{"domain_id": id, "deleted_at": nil}, options.FindOne().SetProjection(sheetAuditProjection)).Decode(&sheet); err != nil {
			return err
		}
		if err := s.checkSheet(ctx, sheet); err != nil {
			return err
		}
		proof = sheet.DurableAcceptance.EventEvidence
	case eventcatalog.InterpretationReportGenerated:
		var p eventoutcome.ReportGeneratedPayload
		if err := json.Unmarshal(inner.Data, &p); err != nil {
			return conflict(err)
		}
		id, err := strconv.ParseUint(p.GenerationID, 10, 64)
		if err != nil || id == 0 {
			return driftf("invalid generation identity")
		}
		var g interpretmongo.ReportGenerationPO
		if err := s.db.Collection("report_generations").FindOne(ctx, bson.M{"domain_id": id, "deleted_at": nil}, options.FindOne().SetProjection(generationAuditProjection)).Decode(&g); err != nil {
			return err
		}
		if err := s.checkGenerated(ctx, g); err != nil {
			return err
		}
		proof = g.GeneratedEventEvidence
	case eventcatalog.InterpretationRetryRequested:
		var p eventoutcome.InterpretationRetryRequestedPayload
		if err := json.Unmarshal(inner.Data, &p); err != nil {
			return conflict(err)
		}
		id, err := strconv.ParseUint(p.RunID, 10, 64)
		if err != nil || id == 0 {
			return driftf("invalid run identity")
		}
		var r interpretmongo.InterpretationRunPO
		if err := s.db.Collection("interpretation_runs").FindOne(ctx, bson.M{"domain_id": id, "deleted_at": nil}, options.FindOne().SetProjection(runAuditProjection)).Decode(&r); err != nil {
			return err
		}
		if err := s.checkRetry(ctx, r); err != nil {
			return err
		}
		proof = r.RetryEventEvidence
	}
	if proof == nil || proof.Class != evidence.StandardReferenceClass || proof.Reference == nil {
		return driftf("standard row has no live reference")
	}
	_, err = verifyRow(row, *proof.Reference)
	return err
}
