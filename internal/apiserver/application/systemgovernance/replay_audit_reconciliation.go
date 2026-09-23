package systemgovernance

import (
	"context"
	"errors"
	"strings"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

// PendingReplayResolver only reads a previously committed authorization
// ledger. A missing result never grants permission to execute it again.
type PendingReplayResolver interface {
	ResolvePending(context.Context, ActionAuditRecord, ReplayPendingInput) ([]outboxport.ManualReplayResult, bool, error)
}

// ReconcilingActionAuditStore closes the window between a committed Outbox
// authorization and its still-running MySQL governance audit. It has no
// authority to replay an unproven request or recover unrelated actions.
type ReconcilingActionAuditStore struct {
	base      ActionAuditStore
	running   RunningActionAuditReader
	resolvers map[string]PendingReplayResolver
}

func NewReconcilingActionAuditStore(base ActionAuditStore, running RunningActionAuditReader, resolvers map[string]PendingReplayResolver) *ReconcilingActionAuditStore {
	copyOfResolvers := make(map[string]PendingReplayResolver, len(resolvers))
	for name, resolver := range resolvers {
		copyOfResolvers[name] = resolver
	}
	return &ReconcilingActionAuditStore{base: base, running: running, resolvers: copyOfResolvers}
}

func (s *ReconcilingActionAuditStore) Claim(ctx context.Context, record ActionAuditRecord) (*ActionAuditReplay, bool, error) {
	if s == nil || s.base == nil {
		return nil, false, errors.New("governance audit store is unavailable")
	}
	prior, claimed, err := s.base.Claim(ctx, record)
	if err != nil || prior != nil || claimed || record.ActionID != "events.replay_pending" {
		return prior, claimed, err
	}
	if s.running == nil {
		return nil, false, nil
	}
	original, found, err := s.running.LoadRunning(ctx, record)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return s.base.Claim(ctx, record)
	}
	var input ReplayPendingInput
	if err := decodeActionInput(original.Input, &input); err != nil {
		return nil, false, err
	}
	if input.Store == "" || strings.TrimSpace(input.Reason) == "" || len(input.Targets) == 0 || len(input.Targets) > 100 {
		return nil, false, errors.New("running replay audit has invalid stored input")
	}
	resolver := s.resolvers[input.Store]
	if resolver == nil {
		return nil, false, nil
	}
	items, exists, err := resolver.ResolvePending(ctx, original, input)
	if err != nil || !exists {
		return nil, false, err
	}
	if len(items) != len(input.Targets) {
		return nil, false, errors.New("durable replay result count differs from audit input")
	}
	authorized := 0
	for index, item := range items {
		if item.EventID != input.Targets[index].EventID {
			return nil, false, errors.New("durable replay result order differs from audit input")
		}
		if item.Authorized {
			authorized++
		}
	}
	result, err := finalizeActionRun(original.RequestID, original.ActionID, original.StartedAt,
		map[string]interface{}{"store": input.Store, "authorized": authorized, "items": items}, nil)
	if err != nil {
		return nil, false, err
	}
	original.FinishedAt, original.Status, original.Result = result.FinishedAt, result.Status, result
	if err := s.base.Complete(ctx, original); err != nil {
		// The first executor may have completed the same request concurrently.
		// Return its durable result if it won; never run authorization again.
		if completed, _, claimErr := s.base.Claim(ctx, record); claimErr == nil && completed != nil {
			return completed, false, nil
		}
		return nil, false, err
	}
	return &ActionAuditReplay{ActionID: original.ActionID, Result: result}, false, nil
}

func (s *ReconcilingActionAuditStore) Complete(ctx context.Context, record ActionAuditRecord) error {
	if s == nil || s.base == nil {
		return errors.New("governance audit store is unavailable")
	}
	return s.base.Complete(ctx, record)
}
