package retrydispatch

import (
	"context"
	"errors"

	root "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

var ErrHandoffIncomplete = errors.New("offline future responsibility handoff is incomplete; preserve partial and unknown outcomes")

// Handoff borrows one explicitly configured original primary and root runner.
// There is deliberately no sender, dispatcher, clock, DDL or business executor.
type Handoff struct {
	store     port.HandoffStore
	root      root.Runner
	authority port.AuthorityValidator
	binding   transaction.Binding
}

// HandoffPreconditions are verified/attested by the offline maintenance host.
// They are not proof of stopped processes and do not grant production approval.
type HandoffPreconditions struct {
	AdmissionClosed, AllLegacyWritersStopped bool
	ExpectedSourceSHA                        string
	ReviewedDigest                           [32]byte
}

type HandoffOutcome struct {
	ScheduleID string `json:"schedule_id"`
	Status     string `json:"status"` // transferred, already_transferred, or unknown
}

func NewHandoff(store port.HandoffStore, runner root.Runner, authority port.AuthorityValidator, binding transaction.Binding) (*Handoff, error) {
	if store == nil || runner == nil || authority == nil || !binding.Valid() ||
		(binding.Owner != "qs.mysql" && binding.Owner != "qs.mongo") || store.Binding() != binding {
		return nil, transaction.ErrBinding
	}
	return &Handoff{store: store, root: runner, authority: authority, binding: binding}, nil
}

func (h *Handoff) Inspect(ctx context.Context, selected []model.Frozen) (model.LegacyInventory, error) {
	if err := model.ValidateHandoffSelection(h.binding, selected); err != nil {
		return model.LegacyInventory{}, err
	}
	reader, ok := h.store.(port.HandoffInspector)
	if !ok {
		return model.LegacyInventory{}, transaction.ErrBinding
	}
	result, err := reader.InspectLegacy(ctx, selected)
	if err != nil {
		return model.LegacyInventory{}, err
	}
	if result.Binding != h.binding {
		return model.LegacyInventory{}, transaction.ErrBinding
	}
	return result, nil
}

// Apply transfers each selected plan in its own original root, stopping at the
// first failure. Earlier confirmations remain explicit; a commit error remains
// unknown. Repeating this metadata operation with the SAME reviewed plan is
// idempotent in TransferLegacy and never authorizes business or message replay.
func (h *Handoff) Apply(ctx context.Context, review model.HandoffReview, p HandoffPreconditions) ([]HandoffOutcome, error) {
	if !review.Valid() || review.Binding() != h.binding || review.SourceSHA() != p.ExpectedSourceSHA ||
		review.Digest() != p.ReviewedDigest || !p.AdmissionClosed || !p.AllLegacyWritersStopped {
		return nil, model.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := h.store.Validate(ctx); err != nil {
		return nil, err
	}
	plans := review.Plans()
	outcomes := make([]HandoffOutcome, 0, len(plans))
	for _, plan := range plans {
		if err := ctx.Err(); err != nil {
			return outcomes, err
		}
		var already, callbackCompleted bool
		err := h.root.WithinTransaction(ctx, func(tx context.Context) error {
			// Mongo may re-enter this database-only callback. Reset transient
			// output and reuse the exact reviewed plan on every attempt.
			callbackCompleted = false
			var transferErr error
			already, transferErr = h.store.TransferLegacy(tx, plan, h.authority)
			callbackCompleted = transferErr == nil
			return transferErr
		})
		out := HandoffOutcome{ScheduleID: plan.ID(), Status: "unknown"}
		if err != nil || !callbackCompleted {
			return append(outcomes, out), errors.Join(ErrHandoffIncomplete, err)
		}
		out.Status = "transferred"
		if already {
			out.Status = "already_transferred"
		}
		outcomes = append(outcomes, out)
	}
	return outcomes, nil
}
