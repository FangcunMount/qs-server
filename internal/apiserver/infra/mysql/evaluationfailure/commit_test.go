package evaluationfailure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventmessage"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	eventport "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
)

type txKey struct{}
type fakeBoundary struct {
	t        *testing.T
	e        transaction.Execution
	local    transaction.LocalOutcome
	executes int
}

func (b *fakeBoundary) Reserve(_ context.Context, q eventport.Request) (transaction.Reservation, error) {
	if b.e.Plan.Valid() {
		return transaction.Reservation{Execution: b.e}, nil
	}
	intents, err := eventmessage.PrepareIntents(q.Events, routes{}, "api-server")
	require.NoError(b.t, err)
	require.Len(b.t, intents, 1)
	in := intents[0].Message.Input()
	wire := in.Payload
	in.Payload = q.Artifact
	m, err := message.New(in)
	require.NoError(b.t, err)
	physical, err := transport.PhysicalTopicV2(in.Destination, transport.TransactionMessageType, "qs.mysql", "m3-1")
	require.NoError(b.t, err)
	plan, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mysql", Scope: in.Scope, OperationID: in.ID, IdempotencyKey: q.Key, InputHash: q.InputHash, Branches: []transaction.BranchInput{{ID: in.ID, Message: m, Wire: wire, PhysicalTopic: physical, RouteRevision: "m3-1"}}})
	require.NoError(b.t, err)
	b.e = transaction.Execution{Plan: plan, Fence: 1, WriterEpoch: "m3-1", Token: [32]byte{1}, LeaseUntil: time.Now().Add(time.Minute)}
	return transaction.Reservation{Execution: b.e, Acquired: true}, nil
}
func (b *fakeBoundary) Find(context.Context, string, string) (transaction.Reservation, error) {
	if !b.e.Plan.Valid() {
		return transaction.Reservation{}, transaction.ErrNotFound
	}
	return transaction.Reservation{Execution: b.e}, nil
}
func (*fakeBoundary) Artifact(p transaction.FrozenPlan) ([]byte, error) {
	return p.Input().Branches[0].Message.Input().Payload, nil
}
func (b *fakeBoundary) Commit(ctx context.Context, _ transaction.Execution, write func(context.Context) error) (transaction.Result, error) {
	b.executes++
	if b.local == transaction.LocalCommitted {
		if err := write(context.WithValue(ctx, txKey{}, true)); err != nil {
			return transaction.Result{Local: transaction.LocalUnknown}, err
		}
	}
	return transaction.Result{Local: b.local}, nil
}
func (b *fakeBoundary) Confirm(context.Context, transaction.Execution) transaction.LocalOutcome {
	return b.local
}

type fakeAssessment struct {
	value  *assessment.Assessment
	writes int
}

func (r *fakeAssessment) Save(ctx context.Context, a *assessment.Assessment) error {
	if ctx.Value(txKey{}) != true {
		return errors.New("original root missing")
	}
	r.value = a
	r.writes++
	return nil
}
func (r *fakeAssessment) FindByID(context.Context, assessment.ID) (*assessment.Assessment, error) {
	return r.value, nil
}
func (*fakeAssessment) FindByAnswerSheetID(context.Context, assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return nil, nil
}
func (*fakeAssessment) Delete(context.Context, assessment.ID) error { return nil }

type fakeRuns struct {
	exact, latest              *evalrun.EvaluationRun
	writes, claims, exactReads int
}

func (*fakeRuns) Claim(context.Context, evaluationrun.ClaimRequest) (evaluationrun.ClaimResult, error) {
	return evaluationrun.ClaimResult{}, errors.New("must not authorize another attempt")
}
func (r *fakeRuns) SaveClaimed(ctx context.Context, run evalrun.EvaluationRun) error {
	if ctx.Value(txKey{}) != true {
		return errors.New("original root missing")
	}
	r.exact = &run
	r.writes++
	return nil
}
func (r *fakeRuns) FindRunByID(_ context.Context, id evalrun.ID) (*evalrun.EvaluationRun, error) {
	r.exactReads++
	if r.exact == nil || r.exact.ID() != id {
		return nil, nil
	}
	return r.exact, nil
}
func (r *fakeRuns) FindLatestByAssessmentID(context.Context, uint64) (*evalrun.EvaluationRun, error) {
	return r.latest, nil
}
func (*fakeRuns) ListByAssessmentID(context.Context, uint64, int) ([]evalrun.EvaluationRun, error) {
	return nil, errors.New("unbounded history enumeration forbidden")
}
func (*fakeRuns) ListRetryableFailed(context.Context, evaluationrun.ListRetryableFailedParams) (*evaluationrun.ListRetryableFailedResult, error) {
	return nil, nil
}
func TestFailureCommitKeepsTerminalFactsAndReadsExactAttempt(t *testing.T) {
	b := &fakeBoundary{t: t, local: transaction.LocalCommitted}
	as := &fakeAssessment{}
	runs := &fakeRuns{}
	c := &Committer{boundary: b, resolver: routes{}, writer: as, runWriter: runs, primary: as, runs: runs, latest: runs}
	q := failureRequest(t, time.Unix(120, 0), false)
	a, r, err := c.CommitFailure(t.Context(), q)
	require.NoError(t, err)
	require.True(t, a.Status().IsFailed())
	require.Equal(t, evalrun.StatusFailed, r.Attempt().Status)
	require.Equal(t, 1, as.writes)
	require.Equal(t, 1, runs.writes)
	next := evalrun.NextEvaluationRun(*r)
	runs.latest = &next
	repeated := failureRequest(t, time.Unix(125, 0), false)
	original, oldRun, err := c.CommitFailure(t.Context(), repeated)
	require.NoError(t, err)
	require.True(t, q.Assessment.FailedAt().Equal(*original.FailedAt()))
	require.Equal(t, r.ID(), oldRun.ID())
	require.NotEqual(t, next.ID(), oldRun.ID())
	require.Equal(t, 1, b.executes)
	require.Equal(t, 1, as.writes)
	require.Equal(t, 1, runs.writes)
	require.Equal(t, 1, runs.exactReads)
}
func TestFailureUnknownDoesNotPublishFactsOrGrantAnotherAttempt(t *testing.T) {
	b := &fakeBoundary{t: t, local: transaction.LocalUnknown}
	as := &fakeAssessment{}
	runs := &fakeRuns{}
	c := &Committer{boundary: b, resolver: routes{}, writer: as, runWriter: runs, primary: as, runs: runs, latest: runs}
	q := failureRequest(t, time.Unix(120, 0))
	a, r, err := c.CommitFailure(t.Context(), q)
	require.ErrorIs(t, err, eventport.ErrUnresolved)
	require.Nil(t, a)
	require.Nil(t, r)
	require.Zero(t, as.writes)
	require.Zero(t, runs.writes)
	_, _, err = c.CommitFailure(t.Context(), failureRequest(t, time.Unix(125, 0)))
	require.ErrorIs(t, err, eventport.ErrUnresolved)
	require.Equal(t, 1, b.executes)
	require.Zero(t, runs.claims)
}
