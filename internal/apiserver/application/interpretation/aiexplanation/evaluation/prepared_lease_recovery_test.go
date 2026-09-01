package evaluation

import (
	"context"
	"errors"
	"testing"
	"time"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestPreparedLeaseRecovererReawakensOnlyEligibleCandidates(t *testing.T) {
	at := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	readerV2 := &expiredPreparationV2ReaderStub{values: []domainevaluation.ExpiredPreparation{
		{RunID: meta.ID(700), InvocationID: "v2-invocation-1", LeaseExpiresAt: at.Add(-time.Minute)},
	}}
	reader := &expiredPreparationReaderStub{values: []domainevaluation.ExpiredPreparation{
		{RunID: meta.ID(701), InvocationID: "invocation-1", LeaseExpiresAt: at.Add(-time.Minute)},
		{RunID: meta.ID(702), InvocationID: "invocation-2", LeaseExpiresAt: at.Add(-time.Minute)},
		{RunID: meta.ID(703), InvocationID: "invocation-3", LeaseExpiresAt: at.Add(-time.Minute)},
	}}
	committer := &preparedRecoveryCommitterStub{errs: map[meta.ID]error{
		meta.ID(702): domainevaluation.ErrConflict,
		meta.ID(703): domainevaluation.ErrRecoveryNotAllowed,
	}}
	committerV2 := &preparedRecoveryCommitterV2Stub{}
	recoverer, err := NewPreparedLeaseRecoverer(reader, committer, readerV2, committerV2)
	if err != nil {
		t.Fatal(err)
	}

	recovered, err := recoverer.RecoverExpiredLeases(context.Background(), at, 20)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 2 || readerV2.limit != 20 || reader.limit != 19 || len(committer.calls) != 3 || len(committerV2.calls) != 1 {
		t.Fatalf("recovered=%d v2-limit=%d v1-limit=%d calls=%d/%d", recovered, readerV2.limit, reader.limit, len(committerV2.calls), len(committer.calls))
	}
	for _, call := range committer.calls {
		if call.actor != preparedLeaseRecoveryActor || call.reason != preparedLeaseRecoveryReason || call.requestID == "" {
			t.Fatalf("invalid recovery audit: %#v", call)
		}
	}
}

func TestPreparedLeaseRecovererStopsOnInfrastructureFailure(t *testing.T) {
	at := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	reader := &expiredPreparationReaderStub{values: []domainevaluation.ExpiredPreparation{{
		RunID: meta.ID(704), InvocationID: "invocation-4", LeaseExpiresAt: at.Add(-time.Minute),
	}}}
	want := errors.New("transaction unavailable")
	committer := &preparedRecoveryCommitterStub{errs: map[meta.ID]error{meta.ID(704): want}}
	recoverer, err := NewPreparedLeaseRecoverer(
		reader, committer, &expiredPreparationV2ReaderStub{}, &preparedRecoveryCommitterV2Stub{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if recovered, err := recoverer.RecoverExpiredLeases(context.Background(), at, 20); recovered != 0 || !errors.Is(err, want) {
		t.Fatalf("recovered=%d err=%v", recovered, err)
	}
}

func TestPreparedLeaseRecoveryRequestIDIsStableAndInvocationScoped(t *testing.T) {
	leaseExpiresAt := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	first := preparedLeaseRecoveryRequestID(meta.ID(705), "invocation-a", leaseExpiresAt)
	if first != preparedLeaseRecoveryRequestID(meta.ID(705), "invocation-a", leaseExpiresAt) {
		t.Fatal("request id is not deterministic")
	}
	if first == preparedLeaseRecoveryRequestID(meta.ID(705), "invocation-b", leaseExpiresAt) ||
		first == preparedLeaseRecoveryRequestID(meta.ID(705), "invocation-a", leaseExpiresAt.Add(time.Second)) || len(first) > 256 {
		t.Fatalf("request id is not safely invocation-scoped: %q", first)
	}
}

type expiredPreparationReaderStub struct {
	values []domainevaluation.ExpiredPreparation
	limit  int
}

type expiredPreparationV2ReaderStub struct {
	values []domainevaluation.ExpiredPreparation
	limit  int
}

func (s *expiredPreparationV2ReaderStub) ListExpiredPreparationsV2(_ context.Context, _ time.Time, limit int) ([]domainevaluation.ExpiredPreparation, error) {
	s.limit = limit
	return s.values, nil
}

func (s *expiredPreparationReaderStub) ListExpiredPreparations(_ context.Context, _ time.Time, limit int) ([]domainevaluation.ExpiredPreparation, error) {
	s.limit = limit
	return s.values, nil
}

type preparedRecoveryCall struct {
	runID        meta.ID
	invocationID string
	requestID    string
	actor        string
	reason       string
}

type preparedRecoveryCommitterStub struct {
	calls []preparedRecoveryCall
	errs  map[meta.ID]error
}

type preparedRecoveryCommitterV2Stub struct {
	calls []preparedRecoveryCall
	errs  map[meta.ID]error
}

func (s *preparedRecoveryCommitterV2Stub) CommitExpiredPreparationRecoveryV2(
	_ context.Context, runID meta.ID, invocationID string, _ time.Time, requestID string,
) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	s.calls = append(s.calls, preparedRecoveryCall{
		runID: runID, invocationID: invocationID, requestID: requestID,
	})
	return nil, s.errs[runID]
}

func (s *preparedRecoveryCommitterStub) CommitExpiredPreparationRecovery(
	_ context.Context, runID meta.ID, invocationID string, _ time.Time, requestID, actor, reason string,
) (*domainevaluation.PromptEvaluationRun, error) {
	s.calls = append(s.calls, preparedRecoveryCall{
		runID: runID, invocationID: invocationID, requestID: requestID, actor: actor, reason: reason,
	})
	return nil, s.errs[runID]
}
