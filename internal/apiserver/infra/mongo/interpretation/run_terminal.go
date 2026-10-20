package interpretation

import (
	"bytes"
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	run "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	guarded "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
)

// FrozenRunTerminal is prepared before entering the driver's transaction
// callback. Its private BSON and proof cannot be changed through caller-owned
// aggregates. Reentry performs the same conditional database write each time.
type FrozenRunTerminal struct {
	filter bson.Raw
	update bson.Raw
	proof  *evidence.EventEvidenceV2
}

// FreezeRunTerminal captures the original worker claim, rather than permitting
// a late worker to overwrite a reclaimed Run by ID alone. The owner's guard
// must already have been acquired when Write is called.
func FreezeRunTerminal(original, terminal *run.InterpretationRun, proof *evidence.EventEvidenceV2) (*FrozenRunTerminal, error) {
	if original == nil || terminal == nil || original.Status() != run.StatusRunning || original.StartedAt() == nil ||
		original.ID() != terminal.ID() || original.GenerationID() != terminal.GenerationID() || original.Attempt() != terminal.Attempt() ||
		terminal.FinishedAt() == nil || terminal.LeaseExpiresAt() != nil || proof == nil {
		return nil, transaction.ErrInvalidPlan
	}
	if err := proof.Validate(); err != nil {
		return nil, err
	}
	ref, err := transaction.DecodeReference(proof.TransactionReference)
	if err != nil || ref.Owner != "qs.mongo" || !proof.RecordedAt.UTC().Truncate(time.Millisecond).Equal(terminal.FinishedAt().UTC().Truncate(time.Millisecond)) {
		return nil, transaction.ErrBinding
	}
	switch terminal.Status() {
	case run.StatusSucceeded:
		if proof.Reference.EventType != catalog.InterpretationReportGenerated || terminal.FailedTransactionEvidence() != nil {
			return nil, transaction.ErrBinding
		}
	case run.StatusFailed:
		attached := terminal.FailedTransactionEvidence()
		if !sameTransactionProof(attached, proof) || proof.Reference.EventType != catalog.InterpretationReportFailed {
			return nil, transaction.ErrBinding
		}
	default:
		return nil, transaction.ErrInvalidPlan
	}
	mapper := NewLifecycleMapper()
	before, after := mapper.RunToPO(original), mapper.RunToPO(terminal)
	// Only the terminal transition may differ; ownership/recovery facts are
	// copied from the winning original claim, never recomputed in a retry.
	claim := func(po *InterpretationRunPO) ([]byte, error) {
		return bson.Marshal(bson.D{
			{Key: "trace_id", Value: po.TraceID}, {Key: "started_at", Value: po.StartedAt},
			{Key: "attempt_origin", Value: po.AttemptOrigin}, {Key: "recovery_count", Value: po.RecoveryCount},
			{Key: "last_reclaimed_at", Value: po.LastReclaimedAt}, {Key: "claim_history", Value: po.ClaimHistory},
		})
	}
	a, err := claim(before)
	if err != nil {
		return nil, err
	}
	b, err := claim(after)
	if err != nil || !bytes.Equal(a, b) {
		return nil, transaction.ErrBinding
	}
	filter, err := bson.Marshal(runClaimFilter(before))
	if err != nil {
		return nil, err
	}
	update, err := bson.Marshal(bson.M{"$set": runTerminalSet(after, *terminal.FinishedAt())})
	if err != nil {
		return nil, err
	}
	return &FrozenRunTerminal{filter: filter, update: update, proof: proof.Clone()}, nil
}

func sameTransactionProof(a, b *evidence.EventEvidenceV2) bool {
	return a != nil && b != nil && a.Version == b.Version && a.Class == b.Class && a.Reference == b.Reference &&
		bytes.Equal(a.TransactionReference, b.TransactionReference) && a.BusinessBindingSHA256 == b.BusinessBindingSHA256 &&
		a.Origin == b.Origin && a.RecordedAt.Equal(b.RecordedAt)
}

func runClaimFilter(before *InterpretationRunPO) bson.M {
	filter := bson.M{
		"domain_id": before.DomainID.Uint64(), "generation_id": before.GenerationID,
		"attempt": before.Attempt, "status": string(run.StatusRunning),
		"trace_id": before.TraceID, "started_at": before.StartedAt,
		"lease_expires_at": before.LeaseExpiresAt, "recovery_count": before.RecoveryCount,
	}
	// These fields were historically omitted for their zero values. Match
	// either representation without matching a later nonzero recovery epoch.
	if before.TraceID == "" {
		filter["trace_id"] = bson.M{"$in": bson.A{"", nil}}
	}
	if before.RecoveryCount == 0 {
		filter["recovery_count"] = bson.M{"$in": bson.A{0, nil}}
	}
	return filter
}

func runTerminalSet(po *InterpretationRunPO, at time.Time) bson.M {
	return bson.M{
		"status": po.Status, "failure": po.Failure, "trace_id": po.TraceID,
		"started_at": po.StartedAt, "lease_expires_at": po.LeaseExpiresAt, "finished_at": po.FinishedAt,
		"attempt_origin": po.AttemptOrigin, "retry_disposition": po.RetryDisposition,
		"next_attempt_at": po.NextAttemptAt, "policy_max_attempts": po.PolicyMaxAttempts,
		"retry_policy_version": po.RetryPolicyVersion, "retry_event_id": po.RetryEventID,
		"retry_event_evidence": po.RetryEventEvidence, "failed_transaction_evidence": po.FailedTransactionEvidence,
		"action_request_id": po.ActionRequestID, "recovery_count": po.RecoveryCount,
		"last_reclaimed_at": po.LastReclaimedAt, "claim_history": po.ClaimHistory, "updated_at": at,
	}
}

func (f *FrozenRunTerminal) Write(ctx context.Context, repository *RunRepository) error {
	if f == nil || repository == nil || len(f.filter) == 0 || len(f.update) == 0 {
		return transaction.ErrInvalidPlan
	}
	if err := guarded.RequireOriginal(ctx, repository.DB(), f.proof); err != nil {
		return err
	}
	var filter, update bson.M
	if err := bson.Unmarshal(f.filter, &filter); err != nil {
		return err
	}
	if err := bson.Unmarshal(f.update, &update); err != nil {
		return err
	}
	result, err := repository.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return generation.ErrVersionConflict
	}
	return nil
}
