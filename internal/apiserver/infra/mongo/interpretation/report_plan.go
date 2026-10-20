package interpretation

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	run "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	schedule "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	outcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// The private artifact contains BSON snapshots, not reconstructed consumer
// JSON. No report content is put in metrics or diagnostics. It is never sent
// to consumers; the transport sends the first Branch.Wire unchanged.
type frozenReportCommit struct {
	Version     int
	Generation  []byte
	Original    []byte
	Terminal    []byte
	Report      []byte
	Body        []byte
	At          time.Time
	Association report.Association
	Future      *message.Input
	Due         time.Time
}

func reportCommitKey(id run.ID) string { return "report-terminal/" + id.String() }

func (f frozenReportCommit) records() (*generation.ReportGeneration, *run.InterpretationRun, *run.InterpretationRun, *report.InterpretReport, error) {
	var g ReportGenerationPO
	var original, terminal InterpretationRunPO
	if bson.Unmarshal(f.Generation, &g) != nil || bson.Unmarshal(f.Original, &original) != nil || bson.Unmarshal(f.Terminal, &terminal) != nil {
		return nil, nil, nil, nil, transaction.ErrInvalidPlan
	}
	mapper := NewLifecycleMapper()
	gd, err := mapper.GenerationToDomain(&g)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	a, err := mapper.RunToDomain(&original)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	b, err := mapper.RunToDomain(&terminal)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var artifact *report.InterpretReport
	if len(f.Report) != 0 {
		var po InterpretReportPO
		if bson.Unmarshal(f.Report, &po) != nil || po.ID.IsZero() {
			return nil, nil, nil, nil, transaction.ErrInvalidPlan
		}
		artifact, err = mapper.ReportToDomain(&po)
	}
	return gd, a, b, artifact, err
}

func (f frozenReportCommit) inputHash() ([32]byte, error) {
	g, original, _, _, err := f.records()
	if err != nil {
		return [32]byte{}, err
	}
	// A Run has one terminal operation, including across lease reclaim. New
	// IDs, clocks, report content and failure outcomes cannot replace its first
	// plan. The executor must recover this key before invoking a builder again.
	raw, err := json.Marshal(struct {
		Generation, Run, Outcome, ReportType, Template string
		Attempt                                        int
		Org                                            int64
	}{
		g.ID().String(), original.ID().String(), g.Key().OutcomeID.String(), g.Key().ReportType.String(), g.Key().TemplateVersion.String(), original.Attempt(), f.Association.OrgID,
	})
	return sha256.Sum256(raw), err
}

func freezeReportSuccess(q app.CommitSuccessRequest) (port.Request, error) {
	if q.Generation == nil || q.Run == nil || q.InterpretReport == nil || q.BuilderIdentity == "" || q.ContentSchemaVersion == "" ||
		q.BuilderIdentity != q.InterpretReport.BuilderIdentity() || q.ContentSchemaVersion != q.InterpretReport.ContentSchemaVersion() ||
		(!q.CompletedAt.IsZero() && !q.CompletedAt.Equal(q.InterpretReport.GeneratedAt())) {
		return port.Request{}, transaction.ErrInvalidPlan
	}
	mapper := NewLifecycleMapper()
	r, err := mapper.RunToDomain(mapper.RunToPO(q.Run))
	if err != nil {
		return port.Request{}, err
	}
	at := q.InterpretReport.GeneratedAt()
	if err := r.Succeed(at); err != nil {
		return port.Request{}, err
	}
	po := mapper.ReportToPO(q.InterpretReport)
	po.ID = primitive.NewObjectID()
	po.CreatedAt, po.UpdatedAt = at, at
	evt := reportGeneratedEvent(q.InterpretReport, r.Attempt())
	body, err := json.Marshal(evt.Data)
	if err != nil {
		return port.Request{}, err
	}
	f, err := freezeReportRecords(q.Generation, q.Run, r)
	if err != nil {
		return port.Request{}, err
	}
	f.Report, err = bson.Marshal(po)
	if err != nil {
		return port.Request{}, err
	}
	f.At, f.Association, f.Body = at, q.InterpretReport.Association(), body
	return f.request(evt)
}

func freezeReportFailure(q app.CommitFailureRequest, resolver catalog.TopicResolver) (port.Request, error) {
	if q.Generation == nil || q.Run == nil || q.OutcomeID.IsZero() || q.OutcomeID != q.Generation.Key().OutcomeID || q.FailedAt.IsZero() {
		return port.Request{}, transaction.ErrInvalidPlan
	}
	mapper := NewLifecycleMapper()
	r, err := mapper.RunToDomain(mapper.RunToPO(q.Run))
	if err != nil {
		return port.Request{}, err
	}
	if err := r.Fail(q.FailedAt, q.Failure); err != nil {
		return port.Request{}, err
	}
	var future *message.Input
	var due time.Time
	if decision := r.RetryDecision(); decision != nil && decision.Disposition == retrygovernance.DispositionAutomatic && decision.NextAttemptAt != nil {
		due = *decision.NextAttemptAt
		evt := domain.NewInterpretationRetryRequestedEvent(domain.RetryRequestedEventInput{
			OrgID: q.Association.OrgID, GenerationID: q.Generation.ID().String(), RunID: r.ID().String(),
			AssessmentID: q.Association.AssessmentID.String(), OutcomeID: q.OutcomeID.String(), TesteeID: q.Association.TesteeID,
			ExpectedAttempt: r.Attempt(), AttemptOrigin: string(retrygovernance.AttemptOriginAutomatic), Mode: "next_attempt", RequestedAt: due,
		})
		prepared, err := standard.PrepareIntents([]event.DomainEvent{evt}, resolver, "api-server")
		if err != nil {
			return port.Request{}, err
		}
		in := prepared[0].Message.Input()
		future = &in
		binding, err := eventevidencebinding.Retry(evt.Data)
		if err != nil {
			return port.Request{}, err
		}
		proof, err := evidence.NewStandard(standard.ReferenceFromMessage(prepared[0].Message), binding)
		if err != nil {
			return port.Request{}, err
		}
		if err := r.AttachRetryEventEvidence(proof); err != nil {
			return port.Request{}, err
		}
	}
	evt := reportFailedEvent(q, r)
	body, err := json.Marshal(evt.Data)
	if err != nil {
		return port.Request{}, err
	}
	f, err := freezeReportRecords(q.Generation, q.Run, r)
	if err != nil {
		return port.Request{}, err
	}
	f.Version = 2
	f.At, f.Association, f.Body, f.Future, f.Due = q.FailedAt, q.Association, body, future, due
	return f.request(evt)
}

func freezeReportRecords(g *generation.ReportGeneration, original, terminal *run.InterpretationRun) (frozenReportCommit, error) {
	mapper := NewLifecycleMapper()
	f := frozenReportCommit{Version: 1}
	var err error
	if f.Generation, err = bson.Marshal(mapper.GenerationToPO(g)); err != nil {
		return f, err
	}
	if f.Original, err = bson.Marshal(mapper.RunToPO(original)); err != nil {
		return f, err
	}
	f.Terminal, err = bson.Marshal(mapper.RunToPO(terminal))
	return f, err
}

func (f frozenReportCommit) request(evt event.DomainEvent) (port.Request, error) {
	if err := f.validate(); err != nil {
		return port.Request{}, err
	}
	_, original, _, _, err := f.records()
	if err != nil {
		return port.Request{}, err
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return port.Request{}, err
	}
	hash, err := f.inputHash()
	return port.Request{Key: reportCommitKey(original.ID()), InputHash: hash, Artifact: raw, Events: []event.DomainEvent{evt}}, err
}

func readReportPlan(raw []byte) (frozenReportCommit, error) {
	var f frozenReportCommit
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, err
	}
	canonical, err := json.Marshal(f)
	if err != nil || !bytes.Equal(canonical, raw) {
		return f, transaction.ErrInvalidPlan
	}
	return f, f.validate()
}

func (f frozenReportCommit) validate() error {
	g, original, terminal, artifact, err := f.records()
	if err != nil {
		return err
	}
	if (f.Version != 1 && f.Version != 2) || f.At.IsZero() || f.Association.OrgID <= 0 || f.Association.AssessmentID.IsZero() || f.Association.TesteeID == 0 ||
		g.Status() != generation.StatusGenerating || g.LatestRunID() != original.ID() || g.ID() != original.GenerationID() ||
		original.Status() != run.StatusRunning || original.ID() != terminal.ID() || original.GenerationID() != terminal.GenerationID() || original.Attempt() != terminal.Attempt() ||
		terminal.FinishedAt() == nil || !sameMongoTime(*terminal.FinishedAt(), f.At) || g.GeneratedTransactionEvidence() != nil || terminal.FailedTransactionEvidence() != nil {
		return transaction.ErrInvalidPlan
	}
	if artifact != nil {
		if terminal.Status() != run.StatusSucceeded || artifact.GenerationID() != g.ID() || artifact.InterpretationRunID() != terminal.ID() || artifact.OutcomeID() != g.Key().OutcomeID ||
			artifact.ReportType() != g.Key().ReportType || artifact.TemplateVersion() != g.Key().TemplateVersion || artifact.Association() != f.Association || !sameMongoTime(artifact.GeneratedAt(), f.At) || f.Future != nil || !f.Due.IsZero() {
			return transaction.ErrInvalidPlan
		}
		var body outcome.ReportGeneratedPayload
		if json.Unmarshal(f.Body, &body) != nil {
			return transaction.ErrInvalidPlan
		}
		expected := reportGeneratedPayload(artifact, terminal.Attempt())
		a, err := eventevidencebinding.Generated(body)
		b, other := eventevidencebinding.Generated(expected)
		if err != nil || other != nil || a != b {
			return transaction.ErrInvalidPlan
		}
		return nil
	}
	if terminal.Status() != run.StatusFailed || terminal.Failure() == nil {
		return transaction.ErrInvalidPlan
	}
	var body outcome.ReportFailedPayload
	if json.Unmarshal(f.Body, &body) != nil {
		return transaction.ErrInvalidPlan
	}
	expected := reportFailedPayload(app.CommitFailureRequest{Generation: g, Run: original, OutcomeID: g.Key().OutcomeID, Association: f.Association, Failure: *terminal.Failure(), FailedAt: *terminal.FinishedAt()}, terminal)
	a, err := eventevidencebinding.Failed(body)
	b, other := eventevidencebinding.Failed(expected)
	if err != nil || other != nil || a != b {
		return transaction.ErrInvalidPlan
	}
	d := terminal.RetryDecision()
	if d == nil {
		return transaction.ErrInvalidPlan
	}
	if d.Disposition != retrygovernance.DispositionAutomatic {
		if f.Future != nil || !f.Due.IsZero() || d.NextAttemptAt != nil {
			return transaction.ErrInvalidPlan
		}
		return nil
	}
	if f.Future == nil || d.NextAttemptAt == nil || !sameMongoTime(*d.NextAttemptAt, f.Due) || terminal.RetryEventEvidence() == nil {
		return transaction.ErrInvalidPlan
	}
	m, err := message.New(*f.Future)
	if err != nil {
		return err
	}
	ref := standard.ReferenceFromMessage(m)
	proof := terminal.RetryEventEvidence()
	if proof.Reference == nil || *proof.Reference != ref || proof.EventID != d.RetryEventID || f.Future.Scope != fmt.Sprintf("org:%d", f.Association.OrgID) {
		return transaction.ErrBinding
	}
	envelope, err := standard.VerifyReference(*f.Future, ref.Fingerprint, ref)
	if err != nil {
		return err
	}
	var retry outcome.InterpretationRetryRequestedPayload
	if json.Unmarshal(envelope.Data, &retry) != nil || retry.GenerationID != g.ID().String() || retry.RunID != terminal.ID().String() ||
		retry.OutcomeID != g.Key().OutcomeID.String() || retry.OrgID != f.Association.OrgID || retry.AssessmentID != f.Association.AssessmentID.String() || retry.TesteeID != f.Association.TesteeID ||
		retry.ExpectedAttempt != terminal.Attempt() || retry.AttemptOrigin != string(retrygovernance.AttemptOriginAutomatic) || retry.Mode != "next_attempt" || !retry.RequestedAt.Equal(f.Due) {
		return transaction.ErrInvalidPlan
	}
	binding, err := eventevidencebinding.Retry(retry)
	if err != nil || binding != proof.BusinessBindingSHA256 {
		return transaction.ErrBinding
	}
	return nil
}

func sameMongoTime(a, b time.Time) bool {
	return a.UTC().Truncate(time.Millisecond).Equal(b.UTC().Truncate(time.Millisecond))
}

func reportGeneratedEvent(artifact *report.InterpretReport, attempt int) domain.ReportGeneratedOutcomeEvent {
	return event.New(domain.EventTypeReportGenerated, domain.AggregateType, artifact.GenerationID().String(), reportGeneratedPayload(artifact, attempt))
}

func reportGeneratedPayload(artifact *report.InterpretReport, attempt int) outcome.ReportGeneratedPayload {
	a, content := artifact.Association(), artifact.Content()
	return outcome.ReportGeneratedPayload{
		OrgID: a.OrgID, GenerationID: artifact.GenerationID().String(), RunID: artifact.InterpretationRunID().String(), ReportID: artifact.ID().String(),
		AssessmentID: a.AssessmentID.String(), OutcomeID: artifact.OutcomeID().String(), TesteeID: a.TesteeID, Attempt: uint(attempt),
		ReportType: artifact.ReportType().String(), TemplateVersion: artifact.TemplateVersion().String(), BuilderIdentity: artifact.BuilderIdentity(), ContentSchemaVersion: artifact.ContentSchemaVersion(),
		Model: domain.EventModelIdentityFrom(content.Model), PrimaryScore: domain.EventScoreValueFrom(content.PrimaryScore), Level: domain.EventResultLevelFrom(content.Level), GeneratedAt: artifact.GeneratedAt(),
	}
}

func reportFailedEvent(q app.CommitFailureRequest, terminal *run.InterpretationRun) domain.ReportFailedOutcomeEvent {
	return event.New(domain.EventTypeReportFailed, domain.AggregateType, q.Generation.ID().String(), reportFailedPayload(q, terminal))
}

func reportFailedPayload(q app.CommitFailureRequest, terminal *run.InterpretationRun) outcome.ReportFailedPayload {
	d := terminal.RetryDecision()
	var retry *outcome.RetryDecisionPayload
	if d != nil {
		retry = &outcome.RetryDecisionPayload{Disposition: string(d.Disposition), Retryable: q.Failure.Retryable, Attempt: d.Attempt, MaxAutomaticAttempts: d.MaxAutomaticAttempts, RemainingAutomaticAttempts: d.RemainingAutomaticAttempts,
			NextAttemptAt: d.NextAttemptAt, RetryEventID: d.RetryEventID, ActionRequestID: d.ActionRequestID, PolicyVersion: d.PolicyVersion}
	}
	k := q.Generation.Key()
	return outcome.ReportFailedPayload{
		OrgID: q.Association.OrgID, GenerationID: q.Generation.ID().String(), RunID: terminal.ID().String(), AssessmentID: q.Association.AssessmentID.String(), OutcomeID: k.OutcomeID.String(), TesteeID: q.Association.TesteeID,
		Attempt: uint(terminal.Attempt()), ReportType: k.ReportType.String(), TemplateVersion: k.TemplateVersion.String(), FailureKind: string(q.Failure.Kind), FailureCode: q.Failure.Code,
		Retryable: q.Failure.Retryable, SafeReason: q.Failure.SafeMessage, FailedAt: q.FailedAt, RetryDecision: retry,
	}
}

func (f frozenReportCommit) retryPlan() (schedule.Frozen, error) {
	if f.Version != 2 || f.Future == nil {
		return schedule.Frozen{}, transaction.ErrInvalidPlan
	}
	g, _, terminal, _, err := f.records()
	if err != nil {
		return schedule.Frozen{}, err
	}
	digest, err := schedule.DecisionDigest(terminal.RetryDecision())
	if err != nil {
		return schedule.Frozen{}, err
	}
	return schedule.New(schedule.Authority{Owner: "qs.mongo", OrgID: f.Association.OrgID, Kind: schedule.Interpretation, SubjectID: g.ID().String(), RunID: terminal.ID().String(), AssessmentID: f.Association.AssessmentID.Uint64(), OutcomeID: g.Key().OutcomeID.Uint64(), TesteeID: f.Association.TesteeID, ExpectedAttempt: terminal.Attempt(), Origin: retrygovernance.AttemptOriginAutomatic, DecisionHash: digest}, *f.Future, f.Due)
}
