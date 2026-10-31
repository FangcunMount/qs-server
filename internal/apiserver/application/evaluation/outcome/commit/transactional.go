package commit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventmessage"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

// PendingRecoverer is consulted before claim/reclaim and before model input
// resolution. A persisted result plan owns finalization even after its lease
// expires; it cannot authorize another calculation.
type PendingRecoverer interface {
	RecoverPending(context.Context, *assessment.Assessment) (bool, error)
}

func (c *committer) RecoverPending(ctx context.Context, a *assessment.Assessment) (bool, error) {
	if c.transaction == nil {
		return false, nil
	}
	if a == nil || a.ID().IsZero() {
		return true, port.ErrUnresolved
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	run, err := c.runRepo.FindLatestByAssessmentID(ctx, a.ID().Uint64())
	if err != nil {
		return true, fmt.Errorf("%w: %w", port.ErrUnresolved, err)
	}
	if run == nil {
		return false, nil
	}
	r, err := c.transaction.Find(ctx, fmt.Sprintf("org:%d", a.OrgID()), "evaluation-outcome/"+run.ID().String())
	if errors.Is(err, transaction.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return true, fmt.Errorf("%w: %w", port.ErrUnresolved, err)
	}
	record, _, err := c.recordForExecution(r.Execution)
	if err != nil || record.RunID() != run.ID().String() || record.AssessmentID() != a.ID() || record.OrgID() != a.OrgID() {
		return true, port.ErrUnresolved
	}
	_, err = c.recoverOutcome(ctx, CommitRequest{Assessment: a, Run: run}, record, r.Execution)
	return true, err
}

// NewTransactionalCommitter explicitly selects the final transaction protocol.
// No old stager, fallback provider or Relay wake is part of this boundary.
func NewTransactionalCommitter(boundary port.Committer, assessmentRepo assessment.Repository, outcomeRepo domainoutcome.Repository, runRepo evaluationrun.Repository, projector outcomescoring.Projector) (Committer, error) {
	if boundary == nil || assessmentRepo == nil || outcomeRepo == nil || runRepo == nil {
		return nil, fmt.Errorf("transaction boundary and original evaluation repositories required")
	}
	return &committer{transaction: boundary, assessmentRepo: assessmentRepo, outcomeRepo: outcomeRepo, runRepo: runRepo, scoreProjector: projector, newID: meta.New}, nil
}

type frozenOutcome struct {
	Version              int
	Record               domainoutcome.NewRecordInput
	Payload, ReportInput []byte
}

func outcomeArtifact(in domainoutcome.NewRecordInput) ([]byte, error) {
	f := frozenOutcome{Version: 1, Record: in, Payload: bytes.Clone(in.Payload), ReportInput: bytes.Clone(in.ReportInput)}
	f.Record.Payload, f.Record.ReportInput = nil, nil
	f.Record.CommittedEventEvidence, f.Record.CommittedTransactionEvidence = nil, nil
	return json.Marshal(f)
}

func readOutcomeArtifact(raw []byte) (domainoutcome.NewRecordInput, error) {
	var f frozenOutcome
	if err := json.Unmarshal(raw, &f); err != nil {
		return domainoutcome.NewRecordInput{}, err
	}
	canonical, err := json.Marshal(f)
	if err != nil || !bytes.Equal(raw, canonical) || f.Version != 1 || (len(f.Record.Payload) != 0 && !bytes.Equal(f.Record.Payload, []byte("null"))) || (len(f.Record.ReportInput) != 0 && !bytes.Equal(f.Record.ReportInput, []byte("null"))) || f.Record.CommittedEventEvidence != nil || f.Record.CommittedTransactionEvidence != nil {
		return domainoutcome.NewRecordInput{}, transaction.ErrInvalidPlan
	}
	f.Record.Payload, f.Record.ReportInput = bytes.Clone(f.Payload), bytes.Clone(f.ReportInput)
	if _, err := domainoutcome.NewRecord(f.Record); err != nil {
		return domainoutcome.NewRecordInput{}, err
	}
	return f.Record, nil
}

// The input hash excludes the newly allocated result/event IDs and commit
// clock. Repeating the same Run/result cannot authorize a second execution.
func outcomeRequest(in domainoutcome.NewRecordInput, events []event.DomainEvent) (port.Request, error) {
	raw, err := outcomeArtifact(in)
	if err != nil {
		return port.Request{}, err
	}
	stable := in
	stable.ID, stable.EvaluatedAt = meta.ZeroID, time.Time{}
	input, err := outcomeArtifact(stable)
	if err != nil {
		return port.Request{}, err
	}
	return port.Request{Key: "evaluation-outcome/" + in.RunID, InputHash: sha256.Sum256(input), Artifact: raw, Events: events}, nil
}

func (c *committer) commitTransactional(ctx context.Context, request CommitRequest, candidate domainoutcome.NewRecordInput, prepared *assessment.Assessment, run evalrun.EvaluationRun, completed event.DomainEvent) (committed *domainoutcome.Record, commitErr error) {
	defer func() {
		if commitErr != nil {
			commitErr = fmt.Errorf("%w: %w", port.ErrUnresolved, commitErr)
		}
	}()
	q, err := outcomeRequest(candidate, prepared.Events())
	if err != nil {
		return nil, err
	}
	r, err := c.transaction.Reserve(ctx, q)
	if err != nil {
		return nil, err
	}
	record, raw, err := c.recordForExecution(r.Execution)
	if err != nil {
		return nil, err
	}
	if err := r.Execution.Plan.MatchRequest("qs.mysql", fmt.Sprintf("org:%d", candidate.OrgID), q.Key, q.InputHash); err != nil {
		return nil, err
	}
	if record.RunID() != run.ID().String() || record.AssessmentID() != prepared.ID() || record.OrgID() != prepared.OrgID() {
		return nil, transaction.ErrInputConflict
	}
	refs := r.Execution.References()
	if !r.Acquired {
		return c.recoverOutcome(ctx, request, record, r.Execution)
	}
	// A winner must execute exactly its own frozen candidate. A concurrent
	// loser never substitutes newly generated event IDs or state into it.
	if !bytes.Equal(raw, q.Artifact) || refs[0].MessageID != completed.EventID() {
		return nil, transaction.ErrInvalidPlan
	}
	if err := run.Succeed(record.EvaluatedAt()); err != nil {
		return nil, err
	}
	result, err := c.transaction.Commit(ctx, r.Execution, func(txCtx context.Context) error {
		if err := c.outcomeRepo.Save(txCtx, record); err != nil {
			return err
		}
		if c.scoreProjector != nil {
			if err := c.scoreProjector.Project(txCtx, record, prepared, request.Execution); err != nil {
				return err
			}
		}
		if err := c.assessmentRepo.Save(txCtx, prepared); err != nil {
			return err
		}
		return c.runRepo.SaveClaimed(txCtx, run)
	})
	if result.Local != transaction.LocalCommitted {
		if err != nil {
			return nil, err
		}
		return nil, port.ErrUnresolved
	}
	prepared.ClearEvents()
	*request.Assessment, *request.Run = *prepared, run
	return record, nil
}

func (c *committer) recoverOutcome(ctx context.Context, request CommitRequest, frozen *domainoutcome.Record, execution transaction.Execution) (*domainoutcome.Record, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if c.transaction.Confirm(ctx, execution) != transaction.LocalCommitted {
		return nil, port.ErrUnresolved
	}
	stored, err := c.outcomeRepo.FindByID(ctx, frozen.ID())
	if err != nil {
		return nil, err
	}
	if stored == nil || stored.BusinessBindingSHA256() != frozen.BusinessBindingSHA256() || stored.CommittedTransactionEvidence() == nil || !bytes.Equal(stored.CommittedTransactionEvidence().TransactionReference, frozen.CommittedTransactionEvidence().TransactionReference) {
		return nil, port.ErrUnresolved
	}
	a, err := c.assessmentRepo.FindByID(ctx, stored.AssessmentID())
	if err != nil {
		return nil, err
	}
	run, err := c.runRepo.FindLatestByAssessmentID(ctx, stored.AssessmentID().Uint64())
	if err != nil {
		return nil, err
	}
	if a == nil || !a.Status().IsEvaluated() || a.EvaluatedAt() == nil || !a.EvaluatedAt().Equal(stored.EvaluatedAt()) || run == nil || run.ID().String() != stored.RunID() || run.Attempt().Status != evalrun.StatusSucceeded {
		return nil, port.ErrUnresolved
	}
	// Projection is read from committed state, never recomputed by recovery.
	a.ClearEvents()
	*request.Assessment, *request.Run = *a, *run
	return stored, nil
}

func (c *committer) recordForExecution(execution transaction.Execution) (*domainoutcome.Record, []byte, error) {
	raw, err := c.transaction.Artifact(execution.Plan)
	if err != nil {
		return nil, nil, err
	}
	in, err := readOutcomeArtifact(raw)
	if err != nil {
		return nil, nil, err
	}
	winnerRequest, err := outcomeRequest(in, nil)
	if err != nil || execution.Plan.Input().Owner != "qs.mysql" || execution.Plan.Input().IdempotencyKey != winnerRequest.Key || execution.Plan.Input().InputHash != winnerRequest.InputHash {
		return nil, nil, transaction.ErrInputConflict
	}
	branches, refs := execution.Plan.Input().Branches, execution.References()
	if len(branches) != 1 || len(refs) != 1 || refs[0].Owner != "qs.mysql" || refs[0].Scope != fmt.Sprintf("org:%d", in.OrgID) {
		return nil, nil, transaction.ErrInvalidPlan
	}
	// The private artifact fingerprint and the consumer wire are independent.
	// Bind the latter's typed business payload to the frozen outcome as well.
	consumer := branches[0].Message.Input()
	consumer.Payload = bytes.Clone(branches[0].Wire)
	original, err := message.New(consumer)
	if err != nil {
		return nil, nil, err
	}
	ref := eventmessage.ReferenceFromMessage(original)
	envelope, err := eventmessage.VerifyReference(consumer, ref.Fingerprint, ref)
	if err != nil {
		return nil, nil, err
	}
	var body assessment.EvaluationOutcomeCommittedData
	if json.Unmarshal(envelope.Data, &body) != nil || envelope.AggregateID != in.AssessmentID.String() || body.OrgID != in.OrgID || body.AssessmentID != int64(in.AssessmentID.Uint64()) || body.TesteeID != in.TesteeID || body.OutcomeID != in.ID.String() || body.EvaluationRunID != in.RunID || !body.CommittedAt.Equal(in.EvaluatedAt) {
		return nil, nil, transaction.ErrInvalidPlan
	}
	proof, err := evidence.NewTransactional(eventmessage.ReferenceFromMessage(branches[0].Message), refs[0], domainoutcome.BusinessBindingSHA256(in), in.EvaluatedAt)
	if err != nil {
		return nil, nil, err
	}
	in.CommittedTransactionEvidence = proof
	record, err := domainoutcome.NewRecord(in)
	if err != nil {
		return nil, nil, err
	}
	return record, raw, nil
}
