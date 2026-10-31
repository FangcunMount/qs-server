package retrydispatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventmessage"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	events "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type routes struct{}

func (routes) GetTopicForEvent(string) (string, bool) { return model.LifecycleTopic, true }

type storeFixture struct {
	record        model.Record
	marks, guards int
	block         chan struct{}
	entered       chan struct{}
}

func (s *storeFixture) Validate(context.Context) error { return nil }
func (s *storeFixture) SaveFrozen(context.Context, model.Frozen) (model.Frozen, error) {
	panic("dispatch cannot create business authorization")
}
func (s *storeFixture) Find(context.Context, model.Frozen) (*model.Record, error) {
	r := s.record
	return &r, nil
}
func (s *storeFixture) Due(_ context.Context, at time.Time, _ int) ([]model.Record, error) {
	if s.block != nil {
		close(s.entered)
		<-s.block
	}
	if s.record.State == model.Ready && !s.record.Plan.DueAt().After(at) {
		return []model.Record{s.record}, nil
	}
	return nil, nil
}
func (s *storeFixture) GuardDispatch(ctx context.Context, r model.Record, _ *evidence.EventEvidenceV2) (context.Context, error) {
	s.guards++
	if r.Version != s.record.Version || s.record.State != model.Ready {
		return nil, transaction.ErrStaleExecution
	}
	return ctx, nil
}
func (s *storeFixture) MarkDispatched(_ context.Context, r model.Record, p *evidence.EventEvidenceV2) error {
	s.marks++
	s.record = model.Record{Plan: r.Plan, State: model.DispatchCommitted, Version: r.Version + 1, Proof: p}
	return nil
}

type authorityFixture struct {
	calls int
	err   error
}

func (a *authorityFixture) ValidateDispatchAuthority(context.Context, model.Frozen) error {
	a.calls++
	return a.err
}

type ownerFixture struct {
	execution                 transaction.Execution
	reserveCalls, commitCalls int
	halfUnknown               bool
	local                     transaction.LocalOutcome
	finalUnknown              bool
}

func (o *ownerFixture) Reserve(_ context.Context, q events.Request) (transaction.Reservation, error) {
	o.reserveCalls++
	if o.execution.Plan.Valid() {
		return transaction.Reservation{Execution: o.execution}, nil
	}
	p, err := shared.Freeze(q, routes{}, "qs.mysql", "m3-1")
	if err != nil {
		return transaction.Reservation{}, err
	}
	o.execution = transaction.Execution{Plan: p, Fence: 1, WriterEpoch: "m3-1"}
	return transaction.Reservation{Execution: o.execution, Acquired: true}, nil
}
func (o *ownerFixture) Find(context.Context, string, string) (transaction.Reservation, error) {
	return transaction.Reservation{Execution: o.execution}, nil
}
func (o *ownerFixture) Artifact(p transaction.FrozenPlan) ([]byte, error) {
	return shared.Artifact(p, "qs.mysql", "m3-1")
}
func (o *ownerFixture) Confirm(context.Context, transaction.Execution) transaction.LocalOutcome {
	return o.local
}
func (o *ownerFixture) Commit(ctx context.Context, e transaction.Execution, write func(context.Context) error) (transaction.Result, error) {
	o.commitCalls++
	r := transaction.InitialResult(e)
	if o.halfUnknown {
		return r, errors.New("half response unknown; no local executor")
	}
	if err := write(ctx); err != nil {
		return r, err
	}
	o.local = transaction.LocalCommitted
	r.Local = o.local
	if o.finalUnknown {
		r.Branches[0].Final = transaction.FinalUnknown
	}
	return r, nil
}

func fixture(t *testing.T) (*Dispatcher, *storeFixture, *ownerFixture, *authorityFixture, time.Time) {
	t.Helper()
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	e := event.Event[payload.EvaluationRequestedData]{BaseEvent: event.BaseEvent{ID: "retry-first", EventTypeValue: "evaluation.retry.requested", OccurredAtValue: at, AggregateTypeValue: "Evaluation", AggregateIDValue: "41"}, Data: payload.EvaluationRequestedData{OrgID: 7, AssessmentID: 41, TesteeID: 9, ExpectedAttempt: 1, AttemptOrigin: "automatic", Mode: "next_attempt", RequestedAt: at}}
	prepared, err := eventmessage.PrepareIntents([]event.DomainEvent{e}, routes{}, "api-server")
	require.NoError(t, err)
	f, err := model.New(model.Authority{Owner: "qs.mysql", OrgID: 7, Kind: model.Evaluation, SubjectID: "41", RunID: "41:1", AssessmentID: 41, TesteeID: 9, ExpectedAttempt: 1, Origin: retrygovernance.AttemptOriginAutomatic, DecisionHash: [32]byte{1}}, prepared[0].Message.Input(), at)
	require.NoError(t, err)
	s := &storeFixture{record: model.Record{Plan: f, State: model.Ready, Version: 1}}
	o := &ownerFixture{}
	a := &authorityFixture{}
	d, err := New(s, o, a)
	require.NoError(t, err)
	return d, s, o, a, at
}

func TestNoHalfBeforeDueAndNoLocalWorkOnUnknownPreparation(t *testing.T) {
	d, s, o, a, at := fixture(t)
	outcomes, err := d.Once(t.Context(), at.Add(-time.Nanosecond), 10)
	require.NoError(t, err)
	require.Empty(t, outcomes)
	require.Zero(t, o.reserveCalls)
	o.halfUnknown = true
	outcomes, err = d.Once(t.Context(), at, 10)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	require.ErrorIs(t, outcomes[0].Err, events.ErrUnresolved)
	require.Zero(t, a.calls)
	require.Zero(t, s.guards)
	require.Zero(t, s.marks)
	require.Equal(t, model.Ready, s.record.State)
	// A repeat is recovery only, not another callback or implicit new fence.
	outcomes, err = d.Once(t.Context(), at.Add(time.Hour), 10)
	require.NoError(t, err)
	require.ErrorIs(t, outcomes[0].Err, events.ErrUnresolved)
	require.Equal(t, 1, o.commitCalls)
}

func TestCommittedFinalUnknownDoesNotRepeatDispatchOrRegenerateWire(t *testing.T) {
	d, s, o, a, at := fixture(t)
	first := s.record.Plan.Bytes()
	wire := s.record.Plan.Message().Input().Payload
	o.finalUnknown = true
	outcomes, err := d.Once(t.Context(), at, 10)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	require.NoError(t, outcomes[0].Err)
	require.Equal(t, transaction.LocalCommitted, outcomes[0].Transaction.Local)
	require.Equal(t, transaction.FinalUnknown, outcomes[0].Transaction.Branches[0].Final)
	require.Equal(t, model.DispatchCommitted, s.record.State)
	require.Equal(t, first, s.record.Plan.Bytes())
	require.Equal(t, wire, o.execution.Plan.Input().Branches[0].Wire)
	require.Equal(t, 1, a.calls)
	require.Equal(t, 1, s.marks)
	outcomes, err = d.Once(t.Context(), at.Add(time.Hour), 10)
	require.NoError(t, err)
	require.Empty(t, outcomes)
	require.Equal(t, 1, o.reserveCalls)
	require.Equal(t, 1, o.commitCalls)
}

func TestChangedAuthorityAbortsDispatchAndNeverMarksScheduleCommitted(t *testing.T) {
	d, s, _, a, at := fixture(t)
	a.err = transaction.ErrStaleExecution
	outcomes, err := d.Once(t.Context(), at, 1)
	require.NoError(t, err)
	require.ErrorIs(t, outcomes[0].Err, events.ErrUnresolved)
	require.Equal(t, 1, a.calls)
	require.Zero(t, s.marks)
	require.Equal(t, model.Ready, s.record.State)
}

func TestScanAdmissionIsSerialBoundedAndDoesNotSpawnRejectedWork(t *testing.T) {
	d, s, _, _, at := fixture(t)
	_, err := d.Once(t.Context(), at, 101)
	require.ErrorIs(t, err, model.ErrInvalid)
	s.block = make(chan struct{})
	s.entered = make(chan struct{})
	done := make(chan error, 1)
	go func() { _, err := d.Once(context.Background(), at, 1); done <- err }()
	<-s.entered
	_, err = d.Once(t.Context(), at, 1)
	require.ErrorIs(t, err, ErrBusy)
	close(s.block)
	require.NoError(t, <-done)
}

// A protocol retry may advance only an already Aborted execution. This fake
// deliberately exposes a separate method so an ordinary scan cannot call it.
type retryOwnerFixture struct {
	*ownerFixture
	retries  int
	acquired bool
	corrupt  bool
}

func (o *retryOwnerFixture) RetryAborted(_ context.Context, previous transaction.Execution, options transaction.ReserveOptions) (transaction.Reservation, error) {
	o.retries++
	next := previous
	next.Fence++
	next.Token = [32]byte{9}
	next.LeaseUntil = time.Now().Add(options.Lease)
	if o.corrupt {
		next.Fence++
	}
	o.execution = next
	return transaction.Reservation{Execution: next, Acquired: o.acquired}, nil
}

func TestExplicitAbortedRetryPreservesFrozenPlanAndOriginalWire(t *testing.T) {
	d, s, o, a, at := fixture(t)
	q, err := model.FreezeDispatch(s.record, at)
	require.NoError(t, err)
	previous, err := o.Reserve(t.Context(), q)
	require.NoError(t, err)
	// A fresh instance's readonly execution has no winner capability.
	require.False(t, previous.Execution.Valid())
	o.local = transaction.LocalAborted
	retry := &retryOwnerFixture{ownerFixture: o, acquired: true}
	d.owner = retry
	record := s.record
	original := previous.Execution.Plan.Digest()
	out := d.RetryAborted(t.Context(), record, previous.Execution, time.Minute)
	require.NoError(t, out.Err)
	require.Equal(t, model.DispatchCommitted, out.State)
	require.Equal(t, transaction.LocalCommitted, out.Transaction.Local)
	require.Equal(t, uint64(2), o.execution.Fence)
	require.Equal(t, original, o.execution.Plan.Digest())
	require.Equal(t, record.Plan.Message().Input().Payload, o.execution.Plan.Input().Branches[0].Wire)
	require.Equal(t, 1, retry.retries)
	require.Equal(t, 1, o.commitCalls)
	require.Equal(t, 1, a.calls)
	// A caller holding the old Ready snapshot cannot create another execution.
	out = d.RetryAborted(t.Context(), record, previous.Execution, time.Minute)
	require.ErrorIs(t, out.Err, transaction.ErrStaleExecution)
	require.Equal(t, 1, retry.retries)
}

func TestExplicitRetryRejectsUnprovedOrChangedExecution(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*storeFixture, *ownerFixture, *retryOwnerFixture)
	}{
		{"unknown", func(_ *storeFixture, o *ownerFixture, _ *retryOwnerFixture) { o.local = transaction.LocalUnknown }},
		{"committed", func(_ *storeFixture, o *ownerFixture, _ *retryOwnerFixture) { o.local = transaction.LocalCommitted }},
		{"held", func(s *storeFixture, _ *ownerFixture, _ *retryOwnerFixture) { s.record.State = model.Held }},
		{"stale-revision", func(s *storeFixture, _ *ownerFixture, _ *retryOwnerFixture) { s.record.Version++ }},
		{"lost-new-fence", func(_ *storeFixture, _ *ownerFixture, r *retryOwnerFixture) { r.acquired = false }},
		{"wrong-new-fence", func(_ *storeFixture, _ *ownerFixture, r *retryOwnerFixture) { r.corrupt = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, s, o, a, at := fixture(t)
			record := s.record
			q, err := model.FreezeDispatch(record, at)
			require.NoError(t, err)
			previous, err := o.Reserve(t.Context(), q)
			require.NoError(t, err)
			o.local = transaction.LocalAborted
			retry := &retryOwnerFixture{ownerFixture: o, acquired: true}
			d.owner = retry
			tc.change(s, o, retry)
			out := d.RetryAborted(t.Context(), record, previous.Execution, time.Minute)
			require.Error(t, out.Err)
			require.Zero(t, o.commitCalls)
			require.Zero(t, s.guards)
			require.Zero(t, s.marks)
			require.Zero(t, a.calls)
		})
	}
}

func TestAutomaticPausePreventsReserveAndExplicitProtocolRetry(t *testing.T) {
	d, s, o, _, at := fixture(t)
	q, err := model.FreezeDispatch(s.record, at)
	require.NoError(t, err)
	previous, err := o.Reserve(t.Context(), q)
	require.NoError(t, err)
	o.reserveCalls = 0
	o.local = transaction.LocalAborted
	retry := &retryOwnerFixture{ownerFixture: o, acquired: true}
	d.owner = retry
	WithAutomaticRetrySwitch(func() bool { return true })(d)
	outcomes, err := d.Once(t.Context(), at, 1)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	require.ErrorIs(t, outcomes[0].Err, eventruntime.ErrAutomaticRetryPaused)
	out := d.RetryAborted(t.Context(), s.record, previous.Execution, time.Minute)
	require.ErrorIs(t, out.Err, eventruntime.ErrAutomaticRetryPaused)
	require.Zero(t, o.reserveCalls)
	require.Zero(t, retry.retries)
	require.Zero(t, o.commitCalls)
	require.Equal(t, model.Ready, s.record.State)
	// Resuming the switch preserves the old plan. A scan still cannot retry an
	// Aborted execution; the host must explicitly authorize that protocol action.
	WithAutomaticRetrySwitch(func() bool { return false })(d)
	outcomes, err = d.Once(t.Context(), at, 1)
	require.NoError(t, err)
	require.ErrorIs(t, outcomes[0].Err, events.ErrUnresolved)
	require.Zero(t, retry.retries)
	require.Zero(t, o.commitCalls)
}
