package interpretation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	run "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	schedulemodel "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	guarded "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	schedulestore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retryschedule"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	outcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"github.com/FangcunMount/reliable-messaging/message"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// TransactionCommitter borrows the one qs.mongo owner. It neither starts nor
// closes that owner's runtime, and never installs collections or indexes.
type TransactionCommitter struct {
	boundary   port.Committer
	resolver   catalog.TopicResolver
	generation *GenerationRepository
	runs       *RunRepository
	reports    *ReportRepository
	catalog    *ReportCatalogProjector
	future     *mongo.Collection
	schedules  *schedulestore.Store
}

var _ app.InterpretationCommitter = (*TransactionCommitter)(nil)

func NewTransactionCommitter(db *mongo.Database, owner *guarded.Owner, runner guarded.OriginalRunner, resolver catalog.TopicResolver, opts ...base.BaseRepositoryOptions) (*TransactionCommitter, error) {
	boundary, err := guarded.NewCommitter(db, owner, runner, resolver)
	if err != nil {
		return nil, err
	}
	for _, kind := range []string{catalog.InterpretationReportGenerated, catalog.InterpretationReportFailed} {
		topic, ok := resolver.GetTopicForEvent(kind)
		if !ok || !owner.HasTopic(topic) {
			return nil, transaction.ErrBinding
		}
	}
	primary := db.Client().Database(db.Name(), options.Database().SetReadPreference(readpref.Primary()).SetReadConcern(readconcern.Majority()))
	schedules, err := schedulestore.New(db, owner.Binding(), opts...)
	if err != nil {
		return nil, err
	}
	return &TransactionCommitter{schedules: schedules, boundary: boundary, resolver: resolver, generation: NewGenerationRepositoryWithoutSchema(primary, opts...), runs: NewRunRepositoryWithoutSchema(primary, opts...),
		reports: NewReportRepositoryWithoutSchema(primary, opts...), catalog: NewReportCatalogProjectorWithoutSchema(primary, opts...), future: db.Collection("rm_outbox")}, nil
}

func (c *TransactionCommitter) CommitSuccess(ctx context.Context, q app.CommitSuccessRequest) (*app.CommitResult, error) {
	request, err := freezeReportSuccess(q)
	if err != nil {
		return nil, err
	}
	return c.commit(ctx, request)
}

func (c *TransactionCommitter) CommitFailure(ctx context.Context, q app.CommitFailureRequest) (*app.CommitResult, error) {
	request, err := freezeReportFailure(q, c.resolver)
	if err != nil {
		return nil, err
	}
	return c.commit(ctx, request)
}

func (c *TransactionCommitter) readExecution(e transaction.Execution) (frozenReportCommit, error) {
	raw, err := c.boundary.Artifact(e.Plan)
	if err != nil {
		return frozenReportCommit{}, err
	}
	f, err := readReportPlan(raw)
	if err != nil {
		return f, err
	}
	_, original, _, artifact, err := f.records()
	if err != nil {
		return f, err
	}
	hash, err := f.inputHash()
	in := e.Plan.Input()
	if err != nil || len(in.Branches) != 1 || in.Owner != "qs.mongo" || in.Scope != fmt.Sprintf("org:%d", f.Association.OrgID) || in.IdempotencyKey != reportCommitKey(original.ID()) || in.InputHash != hash {
		return f, transaction.ErrInvalidPlan
	}
	b := in.Branches[0]
	consumer := b.Message.Input()
	consumer.Payload = bytes.Clone(b.Wire)
	m, err := message.New(consumer)
	if err != nil {
		return f, err
	}
	ref := standard.ReferenceFromMessage(m)
	envelope, err := standard.VerifyReference(consumer, ref.Fingerprint, ref)
	if err != nil {
		return f, err
	}
	kind := catalog.InterpretationReportFailed
	if artifact != nil {
		kind = catalog.InterpretationReportGenerated
	}
	g, _, _, _, err := f.records()
	if err != nil || consumer.EventType != kind || envelope.AggregateType != domain.AggregateType || envelope.AggregateID != g.ID().String() || !bytes.Equal(envelope.Data, f.Body) {
		return f, transaction.ErrInvalidPlan
	}
	return f, nil
}

func reportPlanProof(f frozenReportCommit, e transaction.Execution) (*evidence.EventEvidenceV2, error) {
	// Lookup intentionally redacts the winner's execution token. Recovery
	// needs the exact immutable reference, never permission to execute again.
	if !e.Plan.Valid() || len(e.References()) != 1 {
		return nil, transaction.ErrInvalidPlan
	}
	b := e.Plan.Input().Branches[0]
	var binding string
	var err error
	if len(f.Report) != 0 {
		var body outcome.ReportGeneratedPayload
		if err := json.Unmarshal(f.Body, &body); err != nil {
			return nil, err
		}
		binding, err = eventevidencebinding.Generated(body)
	} else {
		var body outcome.ReportFailedPayload
		if err := json.Unmarshal(f.Body, &body); err != nil {
			return nil, err
		}
		binding, err = eventevidencebinding.Failed(body)
	}
	if err != nil {
		return nil, err
	}
	ref, err := e.Plan.Reference(b.ID, e.Fence, e.WriterEpoch)
	if err != nil {
		return nil, err
	}
	return evidence.NewTransactional(standard.ReferenceFromMessage(b.Message), ref, binding, f.At.UTC().Truncate(time.Millisecond))
}

func materializeReportCommit(f frozenReportCommit, e transaction.Execution) (*app.CommitResult, *FrozenRunTerminal, error) {
	g, original, terminal, artifact, err := f.records()
	if err != nil {
		return nil, nil, err
	}
	proof, err := reportPlanProof(f, e)
	if err != nil {
		return nil, nil, err
	}
	if artifact != nil {
		err = g.SucceedWithTransactionEvidence(terminal.ID(), artifact.ID(), proof.RecordedAt, proof)
	} else {
		if err = terminal.AttachFailureTransactionEvidence(proof); err == nil {
			err = g.Fail(terminal.ID(), proof.RecordedAt)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	write, err := FreezeRunTerminal(original, terminal, proof)
	return &app.CommitResult{Generation: g, Run: terminal, InterpretReport: artifact}, write, err
}

func (c *TransactionCommitter) commit(ctx context.Context, request port.Request) (*app.CommitResult, error) {
	reservation, err := c.boundary.Reserve(ctx, request)
	if err != nil {
		return nil, errors.Join(port.ErrUnresolved, err)
	}
	e := reservation.Execution
	f, err := c.readExecution(e)
	if err != nil {
		return nil, errors.Join(port.ErrUnresolved, err)
	}
	if err := e.Plan.MatchRequest("qs.mongo", fmt.Sprintf("org:%d", f.Association.OrgID), request.Key, request.InputHash); err != nil {
		return nil, err
	}
	if !reservation.Acquired {
		return c.recover(ctx, e, f)
	}
	raw, err := c.boundary.Artifact(e.Plan)
	if err != nil || !bytes.Equal(raw, request.Artifact) {
		return nil, transaction.ErrInvalidPlan
	}
	prepared, runWrite, err := materializeReportCommit(f, e)
	if err != nil {
		return nil, errors.Join(port.ErrUnresolved, err)
	}
	var future message.Message
	var retryPlan schedulemodel.Frozen
	if f.Future != nil && f.Version == 2 {
		retryPlan, err = f.retryPlan()
		if err != nil {
			return nil, err
		}
	}
	if f.Future != nil && f.Version == 1 {
		future, err = message.New(*f.Future)
		if err != nil {
			return nil, err
		}
	}
	result, err := c.boundary.Commit(ctx, e, func(tx context.Context) error {
		// The SDK has prepared all halves and bound the original guard before
		// entering this callback. Only deterministic database operations follow.
		if retryPlan.Valid() {
			winner, err := c.schedules.SaveFrozen(tx, retryPlan)
			if err != nil {
				return err
			}
			if winner.Digest() != retryPlan.Digest() {
				return schedulemodel.ErrConflict
			}
		}
		if err := runWrite.Write(tx, c.runs); err != nil {
			return err
		}
		if err := c.generation.Save(tx, prepared.Generation, prepared.Generation.Version()-1); err != nil {
			return err
		}
		if prepared.InterpretReport != nil {
			if _, err := c.reports.InsertOne(tx, bson.Raw(f.Report)); err != nil {
				return err
			}
			if err := c.projectReport(tx, f.Report); err != nil {
				return err
			}
		}
		if f.Future != nil && f.Version == 1 {
			// Recover the exact pre-T13 artifact's original responsibility. New V2
			// plans never append a future event to the legacy publishing Outbox.
			appender, err := sdkmongo.Bind(mongo.NewSessionContext(tx, mongo.SessionFromContext(tx)), c.future)
			if err != nil {
				return err
			}
			return appender.Append(future, f.Due)
		}
		return nil
	})
	if result.Local != transaction.LocalCommitted {
		return nil, errors.Join(port.ErrUnresolved, err)
	}
	// FinalUnknown is compatible with a proven local commit. Checker owns
	// final Broker resolution; callers must not execute the builder again.
	return prepared, nil
}

func (c *TransactionCommitter) recover(ctx context.Context, e transaction.Execution, f frozenReportCommit) (*app.CommitResult, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if c.boundary.Confirm(ctx, e) != transaction.LocalCommitted {
		return nil, port.ErrUnresolved
	}
	if f.Future != nil && f.Version == 2 {
		plan, err := f.retryPlan()
		if err != nil {
			return nil, port.ErrUnresolved
		}
		record, err := c.schedules.Find(ctx, plan)
		if err != nil || record.Validate() != nil || record.Plan.Digest() != plan.Digest() {
			return nil, errors.Join(port.ErrUnresolved, err)
		}
	}
	expected, _, err := materializeReportCommit(f, e)
	if err != nil {
		return nil, err
	}
	g, err := c.generation.FindByID(ctx, expected.Generation.ID())
	if err != nil {
		return nil, errors.Join(port.ErrUnresolved, err)
	}
	r, err := c.runs.FindByID(ctx, expected.Run.ID())
	if err != nil {
		return nil, errors.Join(port.ErrUnresolved, err)
	}
	if g.LatestRunID() != r.ID() || g.Status() != expected.Generation.Status() || r.Status() != expected.Run.Status() || r.FinishedAt() == nil || !sameMongoTime(*r.FinishedAt(), f.At) {
		return nil, port.ErrUnresolved
	}
	if expected.InterpretReport == nil {
		// A later manual retry authorization may legitimately change the
		// decision. The original failure proof remains immutable and exact.
		if !sameTransactionProof(r.FailedTransactionEvidence(), expected.Run.FailedTransactionEvidence()) {
			return nil, port.ErrUnresolved
		}
		return &app.CommitResult{Generation: g, Run: r}, nil
	}
	if !sameTransactionProof(g.GeneratedTransactionEvidence(), expected.Generation.GeneratedTransactionEvidence()) || g.ReportID() != expected.InterpretReport.ID() {
		return nil, port.ErrUnresolved
	}
	var original bson.Raw
	if err := c.reports.FindOne(ctx, bson.M{"domain_id": g.ReportID().Uint64()}, &original); err != nil {
		return nil, errors.Join(port.ErrUnresolved, err)
	}
	// Compare the first persisted BSON directly. Rebuilding a document can
	// reorder nested map fields and is not proof that frozen bytes changed.
	if !bytes.Equal(original, f.Report) {
		return nil, port.ErrUnresolved
	}
	var actual InterpretReportPO
	if err := bson.Unmarshal(original, &actual); err != nil {
		return nil, err
	}
	artifact, err := c.reports.mapper.ReportToDomain(&actual)
	if err != nil {
		return nil, err
	}
	return &app.CommitResult{Generation: g, Run: r, InterpretReport: artifact}, nil
}

func (c *TransactionCommitter) projectReport(ctx context.Context, raw []byte) error {
	var po InterpretReportPO
	if err := bson.Unmarshal(raw, &po); err != nil {
		return err
	}
	entry := ReportCatalogPO{AssessmentID: po.AssessmentID, OrgID: po.OrgID, TesteeID: po.TesteeID, OutcomeID: po.OutcomeID, GenerationID: po.GenerationID,
		SourceKind: ReportCatalogSourceArtifact, SourceID: po.DomainID.Uint64(), ModelCode: po.ScaleCode, RiskLevel: po.RiskLevel, SortAt: po.GeneratedAt, SortReportID: po.DomainID.Uint64(), UpdatedAt: po.GeneratedAt}
	var current ReportCatalogPO
	err := c.catalog.FindOne(ctx, bson.M{"assessment_id": po.AssessmentID}, &current)
	if errors.Is(err, mongo.ErrNoDocuments) {
		// Freeze the ObjectID with the report before the callback. Unlike a
		// conditional upsert, a stale winner cannot hide DuplicateKey inside a
		// transaction that Mongo has already aborted.
		_, err = c.catalog.InsertOne(ctx, struct {
			ID    any             `bson:"_id"`
			Entry ReportCatalogPO `bson:",inline"`
		}{po.ID, entry})
		return err
	}
	if err != nil {
		return err
	}
	if current.SortAt.After(entry.SortAt) || current.SortAt.Equal(entry.SortAt) && current.SortReportID >= entry.SortReportID {
		return nil
	}
	result, err := c.catalog.UpdateOne(ctx, bson.M{"assessment_id": po.AssessmentID, "sort_at": current.SortAt, "sort_report_id": current.SortReportID}, bson.M{"$set": entry})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return generation.ErrVersionConflict
	}
	return nil
}

// RecoverPending is called before claiming/reclaiming a Run or executing a
// builder. An existing unresolved plan never grants permission to rebuild.
func (c *TransactionCommitter) RecoverPending(ctx context.Context, key generation.Key, orgID int64) (*app.CommitResult, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	g, err := c.generation.FindByKey(ctx, key)
	if errors.Is(err, generation.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, errors.Join(port.ErrUnresolved, err)
	}
	if g.LatestRunID().IsZero() {
		return nil, false, nil
	}
	r, err := c.runs.FindByID(ctx, g.LatestRunID())
	if err != nil {
		return nil, true, errors.Join(port.ErrUnresolved, err)
	}
	// A committed failed Run proceeds only through its existing explicit
	// business retry authorization, not by resuming the old terminal plan.
	if r.Status() == run.StatusFailed && r.FailedTransactionEvidence() == nil {
		return nil, false, nil
	}
	reservation, err := c.boundary.Find(ctx, fmt.Sprintf("org:%d", orgID), reportCommitKey(r.ID()))
	if errors.Is(err, transaction.ErrNotFound) {
		if r.FailedTransactionEvidence() != nil || g.GeneratedTransactionEvidence() != nil {
			return nil, true, port.ErrUnresolved
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, true, errors.Join(port.ErrUnresolved, err)
	}
	f, err := c.readExecution(reservation.Execution)
	if err != nil {
		return nil, true, errors.Join(port.ErrUnresolved, err)
	}
	fg, original, _, _, err := f.records()
	if err != nil || fg.ID() != g.ID() || original.ID() != r.ID() {
		return nil, true, port.ErrUnresolved
	}
	result, err := c.recover(ctx, reservation.Execution, f)
	if err == nil && r.Status() == run.StatusFailed {
		return nil, false, nil
	}
	return result, true, err
}
