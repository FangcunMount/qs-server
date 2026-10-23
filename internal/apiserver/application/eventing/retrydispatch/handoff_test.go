package retrydispatch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	root "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type handoffRootKey struct{}

type handoffStoreFixture struct {
	*storeFixture
	binding          transaction.Binding
	validated, calls int
	validateErr, err error
	already          bool
	plans            [][]byte
}

func (s *handoffStoreFixture) Binding() transaction.Binding { return s.binding }

func (s *handoffStoreFixture) Validate(context.Context) error {
	s.validated++
	return s.validateErr
}

func (s *handoffStoreFixture) TransferLegacy(ctx context.Context, plan model.Frozen, authority port.AuthorityValidator) (bool, error) {
	if ctx.Value(handoffRootKey{}) != true || authority == nil {
		return false, transaction.ErrTransactionRequired
	}
	s.calls++
	s.plans = append(s.plans, plan.Bytes())
	return s.already, s.err
}

func handoffFixture(t *testing.T) (*handoffStoreFixture, model.HandoffReview, HandoffPreconditions) {
	t.Helper()
	_, store, _, _, _ := fixture(t)
	binding := transaction.Binding{Owner: "qs.mysql", StoreID: "original-primary", WriterEpoch: "m3-1"}
	review, err := model.ReviewHandoff(binding, strings.Repeat("a", 40), "operator:9", "approved maintenance", []model.Frozen{store.record.Plan})
	require.NoError(t, err)
	return &handoffStoreFixture{storeFixture: store, binding: binding}, review, HandoffPreconditions{
		AdmissionClosed: true, AllLegacyWritersStopped: true,
		ExpectedSourceSHA: review.SourceSHA(), ReviewedDigest: review.Digest(),
	}
}

func handoffRunner(ctx context.Context, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, handoffRootKey{}, true))
}

func TestOfflineHandoffRequiresReviewedExactHostBeforeAnyDatabaseWork(t *testing.T) {
	for _, change := range []struct {
		name string
		edit func(*HandoffPreconditions)
	}{
		{"admission", func(p *HandoffPreconditions) { p.AdmissionClosed = false }},
		{"writers", func(p *HandoffPreconditions) { p.AllLegacyWritersStopped = false }},
		{"source", func(p *HandoffPreconditions) { p.ExpectedSourceSHA = strings.Repeat("b", 40) }},
		{"digest", func(p *HandoffPreconditions) { p.ReviewedDigest = [32]byte{1} }},
	} {
		t.Run(change.name, func(t *testing.T) {
			store, review, p := handoffFixture(t)
			h, err := NewHandoff(store, root.RunnerFunc(handoffRunner), &authorityFixture{}, review.Binding())
			require.NoError(t, err)
			change.edit(&p)
			out, err := h.Apply(t.Context(), review, p)
			require.ErrorIs(t, err, model.ErrInvalid)
			require.Empty(t, out)
			require.Zero(t, store.validated)
			require.Zero(t, store.calls)
		})
	}
	store, review, _ := handoffFixture(t)
	binding := review.Binding()
	binding.WriterEpoch = "another-epoch"
	h, err := NewHandoff(store, root.RunnerFunc(handoffRunner), &authorityFixture{}, binding)
	require.ErrorIs(t, err, transaction.ErrBinding)
	require.Nil(t, h)
	require.Zero(t, store.validated)
}

func TestOfflineHandoffRetainsFirstBytesAcrossDatabaseCallbackReentry(t *testing.T) {
	store, review, p := handoffFixture(t)
	runner := root.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		// Simulates only a driver's transaction callback re-entry, not a
		// business retry or real database commit proof.
		if err := handoffRunner(ctx, fn); err != nil {
			return err
		}
		return handoffRunner(ctx, fn)
	})
	h, err := NewHandoff(store, runner, &authorityFixture{}, review.Binding())
	require.NoError(t, err)
	out, err := h.Apply(t.Context(), review, p)
	require.NoError(t, err)
	require.Equal(t, []HandoffOutcome{{ScheduleID: review.Plans()[0].ID(), Status: "transferred"}}, out)
	require.Equal(t, 2, store.calls)
	for _, raw := range store.plans {
		require.Equal(t, review.Plans()[0].Bytes(), raw)
	}
	store.already = true
	out, err = h.Apply(t.Context(), review, p)
	require.NoError(t, err)
	require.Equal(t, "already_transferred", out[0].Status)
}

func TestOfflineHandoffDoesNotTurnCommitReplyLossIntoSuccess(t *testing.T) {
	store, review, p := handoffFixture(t)
	commitUnknown := errors.New("controlled commit reply loss")
	runner := root.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		if err := handoffRunner(ctx, fn); err != nil {
			return err
		}
		return commitUnknown
	})
	h, err := NewHandoff(store, runner, &authorityFixture{}, review.Binding())
	require.NoError(t, err)
	out, err := h.Apply(t.Context(), review, p)
	require.ErrorIs(t, err, ErrHandoffIncomplete)
	require.ErrorIs(t, err, commitUnknown)
	require.Equal(t, "unknown", out[0].Status)
	require.Equal(t, 1, store.calls)
}

func TestOfflineHandoffRejectsMissingRootAndFailedSchemaWithoutTransfer(t *testing.T) {
	store, review, p := handoffFixture(t)
	_, err := NewHandoff(store, nil, &authorityFixture{}, review.Binding())
	require.ErrorIs(t, err, transaction.ErrBinding)
	store.validateErr = transaction.ErrSchema
	h, err := NewHandoff(store, root.RunnerFunc(handoffRunner), &authorityFixture{}, review.Binding())
	require.NoError(t, err)
	out, err := h.Apply(t.Context(), review, p)
	require.ErrorIs(t, err, transaction.ErrSchema)
	require.Empty(t, out)
	require.Zero(t, store.calls)
	store.validateErr = nil
	h.root = root.RunnerFunc(func(context.Context, func(context.Context) error) error { return nil })
	out, err = h.Apply(t.Context(), review, p)
	require.ErrorIs(t, err, ErrHandoffIncomplete)
	require.Equal(t, "unknown", out[0].Status)
	require.Zero(t, store.calls)
}

func TestOfflineHandoffRetainsConfirmedPrefixAndStopsAtFirstUnknown(t *testing.T) {
	store, review, p := handoffFixture(t)
	first := review.Plans()[0]
	plans := []model.Frozen{first}
	for _, id := range []uint64{42, 43} {
		a := first.Authority()
		a.SubjectID, a.RunID, a.AssessmentID = fmt.Sprint(id), fmt.Sprintf("%d:1", id), id
		e := event.Event[payload.EvaluationRequestedData]{
			BaseEvent: event.BaseEvent{ID: fmt.Sprintf("original-%d", id), EventTypeValue: "evaluation.retry.requested", OccurredAtValue: first.DueAt(), AggregateTypeValue: "Evaluation", AggregateIDValue: a.SubjectID},
			Data:      payload.EvaluationRequestedData{OrgID: a.OrgID, AssessmentID: int64(id), TesteeID: a.TesteeID, ExpectedAttempt: 1, AttemptOrigin: "automatic", Mode: "next_attempt", RequestedAt: first.DueAt()},
		}
		prepared, err := standard.PrepareIntents([]event.DomainEvent{e}, routes{}, "api-server")
		require.NoError(t, err)
		plan, err := model.New(a, prepared[0].Message.Input(), first.DueAt())
		require.NoError(t, err)
		plans = append(plans, plan)
	}
	review, err := model.ReviewHandoff(review.Binding(), review.SourceSHA(), review.Actor(), review.Reason(), plans)
	require.NoError(t, err)
	p.ReviewedDigest = review.Digest()
	roots := 0
	runner := root.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		roots++
		if err := handoffRunner(ctx, fn); err != nil {
			return err
		}
		if roots == 2 {
			return errors.New("controlled second commit reply loss")
		}
		return nil
	})
	h, err := NewHandoff(store, runner, &authorityFixture{}, review.Binding())
	require.NoError(t, err)
	out, err := h.Apply(t.Context(), review, p)
	require.ErrorIs(t, err, ErrHandoffIncomplete)
	require.Equal(t, []HandoffOutcome{
		{ScheduleID: plans[0].ID(), Status: "transferred"},
		{ScheduleID: plans[1].ID(), Status: "unknown"},
	}, out)
	require.Equal(t, 2, roots)
	require.Equal(t, 2, store.calls)

	// Cancellation after a confirmed root cannot erase that result or start
	// the next plan. It does not itself prove any rollback.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	h.root = root.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		if err := handoffRunner(ctx, fn); err != nil {
			return err
		}
		cancel()
		return nil
	})
	out, err = h.Apply(ctx, review, p)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []HandoffOutcome{{ScheduleID: first.ID(), Status: "transferred"}}, out)
	require.Equal(t, 3, store.calls)
}
