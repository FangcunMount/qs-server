package interpretation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// RetirementEvidenceStore borrows both Mongo and the immutable Outcome reader.
// It creates no pool, index, transaction, historical scanner or event.
type RetirementEvidenceStore struct {
	db       *mongo.Database
	outcomes evaluationfact.Repository
}

func NewRetirementEvidenceStore(db *mongo.Database, outcomes evaluationfact.Repository) (*RetirementEvidenceStore, error) {
	if db == nil || outcomes == nil || (reflect.ValueOf(outcomes).Kind() == reflect.Pointer && reflect.ValueOf(outcomes).IsNil()) {
		return nil, fmt.Errorf("%w: Mongo database and immutable Outcome reader are required", retirementevidence.ErrUnverifiable)
	}
	return &RetirementEvidenceStore{db: db, outcomes: outcomes}, nil
}

type retirementDependency struct {
	snapshot   *retirementevidence.Snapshot
	collection string
}
type retirementBaseline struct {
	snapshot                          *retirementevidence.Snapshot
	dependencies                      []retirementDependency
	eventID, binding, outcomeIdentity string
	outcomeID                         uint64
}

func (b *retirementBaseline) EventID() string {
	if b == nil {
		return ""
	}
	return b.eventID
}
func (b *retirementBaseline) BindingSHA256() string {
	if b == nil {
		return ""
	}
	return b.binding
}
func (b *retirementBaseline) ValidateEvidence(proof *evidence.EventEvidenceV1) error {
	if b == nil {
		return retirementevidence.ErrUnverifiable
	}
	return retirementevidence.ValidateHistorical(proof, b.eventID, b.binding)
}

type GeneratedRetirementEvidenceBaseline struct {
	retirementBaseline
	payload eventoutcome.ReportGeneratedPayload
}

// OriginalSourceReference is the row verifier's explicit attestation, not a new
// ID generator. Its digest describes the selected historical source bytes and
// must never masquerade as a standard SDK fingerprint.
type OriginalSourceReference struct {
	EventID                      string
	Digest                       evidence.Digest
	BusinessBindingSHA256        string
	Method, Version, OperationID string
}

func (b *GeneratedRetirementEvidenceBaseline) ValidateOriginalEvidence(source OriginalSourceReference, proof *evidence.EventEvidenceV1) error {
	if b == nil {
		return retirementevidence.ErrUnverifiable
	}
	if b.eventID != "" {
		return fmt.Errorf("%w: generation already has an independent original ID", retirementevidence.ErrConflict)
	}
	if err := proof.Validate(); err != nil {
		return err
	}
	if proof.Class != evidence.RetiredVerified && proof.Class != evidence.Unverifiable {
		return fmt.Errorf("maintenance CAS requires a historical conclusion")
	}
	if source.EventID == "" || source.Digest.Kind == "" || source.Digest.Kind == evidence.SDKFingerprintKind || !evidence.ValidSHA256(source.Digest.SHA256) || source.Method == "" || source.Version == "" || source.OperationID == "" {
		return retirementevidence.ErrUnverifiable
	}
	if source.EventID != proof.EventID || source.Digest != proof.Digest || source.BusinessBindingSHA256 != b.binding || source.BusinessBindingSHA256 != proof.BusinessBindingSHA256 || source.Method != proof.Verification.Method || source.Version != proof.Verification.Version || source.OperationID != proof.Verification.OperationID {
		return retirementevidence.ErrConflict
	}
	return nil
}

func (b *GeneratedRetirementEvidenceBaseline) Payload() eventoutcome.ReportGeneratedPayload {
	if b == nil {
		return eventoutcome.ReportGeneratedPayload{}
	}
	out := b.payload
	if out.PrimaryScore != nil {
		value := *out.PrimaryScore
		if value.Max != nil {
			maximum := *value.Max
			value.Max = &maximum
		}
		out.PrimaryScore = &value
	}
	if out.Level != nil {
		value := *out.Level
		out.Level = &value
	}
	return out
}

type RetryRetirementEvidenceBaseline struct {
	retirementBaseline
	payload eventoutcome.InterpretationRetryRequestedPayload
}

func (b *RetryRetirementEvidenceBaseline) Payload() eventoutcome.InterpretationRetryRequestedPayload {
	if b == nil {
		return eventoutcome.InterpretationRetryRequestedPayload{}
	}
	return b.payload
}

func (s *RetirementEvidenceStore) read(ctx context.Context, collection string, id uint64, slot string, target any) (*retirementevidence.Snapshot, error) {
	if s == nil || s.db == nil || id == 0 {
		return nil, retirementevidence.ErrUnverifiable
	}
	snapshot, err := retirementevidence.Read(ctx, s.db.Collection(collection), bson.M{"domain_id": id, "deleted_at": nil}, slot)
	if err != nil {
		return nil, err
	}
	if err := snapshot.Decode(target); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *RetirementEvidenceStore) outcome(ctx context.Context, id uint64) (*evaluationfact.Record, error) {
	if s == nil || s.outcomes == nil || id == 0 {
		return nil, retirementevidence.ErrUnverifiable
	}
	fact, err := s.outcomes.FindByID(ctx, meta.FromUint64(id))
	if err != nil {
		if errors.Is(err, evaluationfact.ErrNotFound) {
			return nil, fmt.Errorf("%w: immutable Outcome is absent: %w", retirementevidence.ErrUnverifiable, err)
		}
		return nil, err
	}
	if fact == nil || fact.ID().Uint64() != id || fact.OrgID() <= 0 || fact.AssessmentID().IsZero() || fact.TesteeID() == 0 {
		return nil, fmt.Errorf("%w: Outcome ownership is incomplete", retirementevidence.ErrUnverifiable)
	}
	return fact, nil
}

// This token covers every public immutable Outcome field, including exact
// selected JSON bytes. It is a private CAS token, not an SDK message fingerprint.
func retirementOutcomeIdentity(fact *evaluationfact.Record) string {
	model, runtime := fact.Model(), fact.Runtime()
	values := []string{fact.ID().String(), strconv.FormatInt(fact.OrgID(), 10), fact.AssessmentID().String(), strconv.FormatUint(fact.TesteeID(), 10), fact.RunID(), string(model.Kind), string(model.Algorithm), model.Code, model.Version, model.Title, string(runtime.DecisionKind), fact.InputSnapshotRef(), strconv.FormatUint(uint64(fact.SchemaVersion()), 10), string(fact.Payload()), string(fact.ReportInput()), fact.EvaluatedAt().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")}
	fields := make([]*string, len(values))
	for i := range values {
		fields[i] = &values[i]
	}
	return evidence.BindingDigest("maintenance.immutable-outcome/v1", fields...)
}

// PrepareGenerated follows the same immutable graph as the live audit. Ordinary
// backfill retains an absent ID; copying a known old source ID requires the
// separate explicit OriginalSourceReference path. No body is rewritten.
func (s *RetirementEvidenceStore) PrepareGenerated(ctx context.Context, id uint64) (*GeneratedRetirementEvidenceBaseline, error) {
	var g ReportGenerationPO
	snapshot, err := s.read(ctx, "report_generations", id, "generated_event_evidence", &g)
	if err != nil {
		return nil, err
	}
	if g.DomainID.Uint64() != id || g.Status != "generated" || g.ReportID == 0 || g.LatestRunID == 0 || g.Version == 0 {
		return nil, fmt.Errorf("%w: generation is not a complete successful terminal", retirementevidence.ErrUnverifiable)
	}
	var artifact InterpretReportPO
	artifactSnapshot, err := s.read(ctx, "interpret_report_artifacts", g.ReportID, "", &artifact)
	if err != nil {
		return nil, err
	}
	var run InterpretationRunPO
	runSnapshot, err := s.read(ctx, "interpretation_runs", g.LatestRunID, "", &run)
	if err != nil {
		return nil, err
	}
	fact, err := s.outcome(ctx, g.OutcomeID)
	if err != nil {
		return nil, err
	}
	if artifact.DomainID.Uint64() != g.ReportID || run.DomainID.Uint64() != g.LatestRunID || artifact.GenerationID != id || artifact.OutcomeID != g.OutcomeID || artifact.InterpretationRunID != g.LatestRunID || run.GenerationID != id || run.Status != "succeeded" || run.Attempt < 1 || artifact.OrgID != fact.OrgID() || artifact.AssessmentID != fact.AssessmentID().Uint64() || artifact.TesteeID != fact.TesteeID() || artifact.ReportType != g.ReportType || artifact.TemplateVersion != g.TemplateVersion || artifact.Model == nil {
		return nil, fmt.Errorf("%w: generated business graph or ownership disagrees", retirementevidence.ErrUnverifiable)
	}
	payload := eventoutcome.ReportGeneratedPayload{OrgID: artifact.OrgID, GenerationID: g.DomainID.String(), RunID: run.DomainID.String(), ReportID: artifact.DomainID.String(), AssessmentID: strconv.FormatUint(artifact.AssessmentID, 10), OutcomeID: strconv.FormatUint(artifact.OutcomeID, 10), TesteeID: artifact.TesteeID, Attempt: uint(run.Attempt), ReportType: artifact.ReportType, TemplateVersion: artifact.TemplateVersion, BuilderIdentity: artifact.BuilderIdentity, ContentSchemaVersion: artifact.ContentSchemaVersion, GeneratedAt: artifact.GeneratedAt, Model: eventoutcome.ModelIdentity{Kind: artifact.Model.Kind, Algorithm: artifact.Model.Algorithm, Code: artifact.Model.Code, Version: artifact.Model.Version, Title: artifact.Model.Title}}
	if value := artifact.PrimaryScore; value != nil {
		payload.PrimaryScore = &eventoutcome.ScoreValue{Kind: value.Kind, Value: value.Value, Label: value.Label, Max: value.Max}
	}
	if value := artifact.Level; value != nil {
		payload.Level = &eventoutcome.ResultLevel{Code: value.Code, Label: value.Label, Severity: value.Severity}
	}
	binding, err := eventevidencebinding.Generated(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", retirementevidence.ErrUnverifiable, err)
	}
	return &GeneratedRetirementEvidenceBaseline{retirementBaseline: retirementBaseline{snapshot: snapshot, dependencies: []retirementDependency{{artifactSnapshot, "interpret_report_artifacts"}, {runSnapshot, "interpretation_runs"}}, eventID: g.GeneratedEventID, binding: binding, outcomeID: g.OutcomeID, outcomeIdentity: retirementOutcomeIdentity(fact)}, payload: payload}, nil
}

func originalRetryOrigin(row InterpretationRunPO) (string, error) {
	if row.RetryEventID == "" {
		return "", fmt.Errorf("%w: original retry ID does not prove its origin", retirementevidence.ErrUnverifiable)
	}
	prefix := fmt.Sprintf("interpret-retry:%d:%d:", row.GenerationID, row.Attempt)
	origin := strings.TrimPrefix(row.RetryEventID, prefix)
	if origin == row.RetryEventID {
		return "", fmt.Errorf("%w: original retry identity disagrees", retirementevidence.ErrUnverifiable)
	}
	if row.ActionRequestID == "" {
		if origin != "automatic" {
			return "", fmt.Errorf("%w: missing original retry action", retirementevidence.ErrUnverifiable)
		}
	} else {
		suffix := ":" + row.ActionRequestID
		if !strings.HasSuffix(origin, suffix) {
			return "", fmt.Errorf("%w: original retry action disagrees", retirementevidence.ErrUnverifiable)
		}
		origin = strings.TrimSuffix(origin, suffix)
		if origin != "manual" && origin != "force" {
			return "", fmt.Errorf("%w: original retry authorization origin disagrees", retirementevidence.ErrUnverifiable)
		}
	}
	return origin, nil
}

// PrepareRetry requires the original scheduling clock and action/origin encoded
// by the stored ID. AttemptOrigin of the failed execution is never reused as
// the origin of the following retry. Active lifecycle responsibility blocks it.
func (s *RetirementEvidenceStore) PrepareRetry(ctx context.Context, id uint64) (*RetryRetirementEvidenceBaseline, error) {
	var run InterpretationRunPO
	snapshot, err := s.read(ctx, "interpretation_runs", id, "retry_event_evidence", &run)
	if err != nil {
		return nil, err
	}
	if run.DomainID.Uint64() != id || run.Status != "failed" || run.Attempt < 1 || run.NextAttemptAt == nil || run.NextAttemptAt.IsZero() || run.RetryDisposition != "automatic" {
		return nil, fmt.Errorf("%w: original retry schedule is incomplete", retirementevidence.ErrUnverifiable)
	}
	origin, err := originalRetryOrigin(run)
	if err != nil {
		return nil, err
	}
	var g ReportGenerationPO
	gSnapshot, err := s.read(ctx, "report_generations", run.GenerationID, "", &g)
	if err != nil {
		return nil, err
	}
	if g.DomainID.Uint64() != run.GenerationID || (g.Status != "generated" && g.Status != "failed") || g.LatestRunID == 0 || g.Version == 0 {
		return nil, fmt.Errorf("%w: generation still has active or unknown responsibility", retirementevidence.ErrUnverifiable)
	}
	var latest InterpretationRunPO
	latestSnapshot, err := s.read(ctx, "interpretation_runs", g.LatestRunID, "", &latest)
	if err != nil {
		return nil, err
	}
	if latest.DomainID.Uint64() != g.LatestRunID || latest.GenerationID != run.GenerationID || (latest.Status != "succeeded" && latest.Status != "failed") || (latest.Status == "failed" && latest.RetryDisposition == "automatic") {
		return nil, fmt.Errorf("%w: retry responsibility is still pending or unknown", retirementevidence.ErrUnverifiable)
	}
	fact, err := s.outcome(ctx, g.OutcomeID)
	if err != nil {
		return nil, err
	}
	payload := eventoutcome.InterpretationRetryRequestedPayload{OrgID: fact.OrgID(), GenerationID: strconv.FormatUint(run.GenerationID, 10), RunID: run.DomainID.String(), AssessmentID: fact.AssessmentID().String(), OutcomeID: fact.ID().String(), TesteeID: fact.TesteeID(), ExpectedAttempt: run.Attempt, AttemptOrigin: origin, ActionRequestID: run.ActionRequestID, Mode: "next_attempt", RequestedAt: *run.NextAttemptAt}
	binding, err := eventevidencebinding.Retry(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", retirementevidence.ErrUnverifiable, err)
	}
	return &RetryRetirementEvidenceBaseline{retirementBaseline: retirementBaseline{snapshot: snapshot, dependencies: []retirementDependency{{gSnapshot, "report_generations"}, {latestSnapshot, "interpretation_runs"}}, eventID: run.RetryEventID, binding: binding, outcomeID: g.OutcomeID, outcomeIdentity: retirementOutcomeIdentity(fact)}, payload: payload}, nil
}

func (s *RetirementEvidenceStore) recheck(ctx context.Context, collection string, baseline *retirementBaseline) error {
	if s == nil || s.db == nil {
		return retirementevidence.ErrUnverifiable
	}
	if err := retirementevidence.RequireTransaction(ctx, s.db.Collection(collection)); err != nil {
		return err
	}
	if baseline == nil {
		return retirementevidence.ErrUnverifiable
	}
	for _, dependency := range baseline.dependencies {
		if err := dependency.snapshot.CheckDependency(ctx, s.db.Collection(dependency.collection)); err != nil {
			return err
		}
	}
	fact, err := s.outcome(ctx, baseline.outcomeID)
	if err != nil {
		return err
	}
	if retirementOutcomeIdentity(fact) != baseline.outcomeIdentity {
		return retirementevidence.ErrConflict
	}
	return nil
}

func (s *RetirementEvidenceStore) backfill(ctx context.Context, collection string, baseline *retirementBaseline, proof *evidence.EventEvidenceV1) error {
	if err := baseline.ValidateEvidence(proof); err != nil {
		return err
	}
	if err := s.recheck(ctx, collection, baseline); err != nil {
		return err
	}
	return baseline.snapshot.Apply(ctx, s.db.Collection(collection), proof)
}

func (s *RetirementEvidenceStore) BackfillGenerated(ctx context.Context, baseline *GeneratedRetirementEvidenceBaseline, proof *evidence.EventEvidenceV1) error {
	if baseline == nil {
		return retirementevidence.ErrUnverifiable
	}
	return s.backfill(ctx, "report_generations", &baseline.retirementBaseline, proof)
}
func (s *RetirementEvidenceStore) BackfillRetry(ctx context.Context, baseline *RetryRetirementEvidenceBaseline, proof *evidence.EventEvidenceV1) error {
	if baseline == nil {
		return retirementevidence.ErrUnverifiable
	}
	return s.backfill(ctx, "interpretation_runs", &baseline.retirementBaseline, proof)
}

// BackfillGeneratedOriginal copies only an explicitly verified historical source
// ID into a generation whose old schema did not store it. A unique ID collision
// or any stale business baseline is returned to the host for transaction abort.
func (s *RetirementEvidenceStore) BackfillGeneratedOriginal(ctx context.Context, baseline *GeneratedRetirementEvidenceBaseline, source OriginalSourceReference, proof *evidence.EventEvidenceV1) error {
	if err := baseline.ValidateOriginalEvidence(source, proof); err != nil {
		return err
	}
	if err := s.recheck(ctx, "report_generations", &baseline.retirementBaseline); err != nil {
		return err
	}
	return baseline.snapshot.ApplyGeneratedOriginal(ctx, s.db.Collection("report_generations"), proof, source.EventID)
}
