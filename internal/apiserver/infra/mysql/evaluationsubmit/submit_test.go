package evaluationsubmit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
)

type rootMarker struct{}
type repoStub struct {
	stored *assessment.Assessment
	writes int
}

func (*repoStub) Save(context.Context, *assessment.Assessment) error {
	return errors.New("submission must retain original conditional writer")
}
func (r *repoStub) SavePendingSubmission(ctx context.Context, a *assessment.Assessment) error {
	if ctx.Value(rootMarker{}) != true {
		return errors.New("original root absent")
	}
	r.writes++
	r.stored = a
	return nil
}
func (r *repoStub) FindByID(context.Context, assessment.ID) (*assessment.Assessment, error) {
	return r.stored, nil
}
func (*repoStub) FindByAnswerSheetID(context.Context, assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return nil, nil
}
func (*repoStub) Delete(context.Context, assessment.ID) error { return nil }

type routes struct{}

func (routes) GetTopicForEvent(string) (string, bool) { return "qs.evaluation.lifecycle", true }

type boundaryStub struct {
	t                  *testing.T
	execution          transaction.Execution
	q                  port.Request
	state              transaction.LocalOutcome
	findErr            error
	reserves, executes int
}

func (b *boundaryStub) Find(context.Context, string, string) (transaction.Reservation, error) {
	if b.findErr != nil {
		return transaction.Reservation{}, b.findErr
	}
	if !b.execution.Plan.Valid() {
		return transaction.Reservation{}, transaction.ErrNotFound
	}
	return transaction.Reservation{Execution: b.execution}, nil
}
func (b *boundaryStub) Reserve(_ context.Context, q port.Request) (transaction.Reservation, error) {
	b.q = q
	b.reserves++
	intents, err := standard.PrepareIntents(q.Events, routes{}, "api-server")
	require.NoError(b.t, err)
	require.Len(b.t, intents, 1)
	in := intents[0].Message.Input()
	wire := in.Payload
	in.Payload = q.Artifact
	m, err := message.New(in)
	require.NoError(b.t, err)
	physical, err := transport.PhysicalTopicV2(in.Destination, transport.TransactionMessageType, "qs.mysql", "m3-1")
	require.NoError(b.t, err)
	p, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mysql", Scope: in.Scope, OperationID: in.ID, IdempotencyKey: q.Key, InputHash: q.InputHash, Branches: []transaction.BranchInput{{ID: in.ID, Message: m, Wire: wire, PhysicalTopic: physical, RouteRevision: "m3-1"}}})
	require.NoError(b.t, err)
	b.execution = transaction.Execution{Plan: p, Fence: 1, WriterEpoch: "m3-1", Token: [32]byte{1}, LeaseUntil: time.Now().Add(time.Minute)}
	return transaction.Reservation{Execution: b.execution, Acquired: true}, nil
}
func (*boundaryStub) Artifact(p transaction.FrozenPlan) ([]byte, error) {
	return p.Input().Branches[0].Message.Input().Payload, nil
}
func (b *boundaryStub) Commit(ctx context.Context, _ transaction.Execution, write func(context.Context) error) (transaction.Result, error) {
	b.executes++
	if b.state == transaction.LocalCommitted {
		if err := write(context.WithValue(ctx, rootMarker{}, true)); err != nil {
			return transaction.Result{Local: transaction.LocalUnknown}, err
		}
	}
	return transaction.Result{Local: b.state}, nil
}
func (b *boundaryStub) Confirm(context.Context, transaction.Execution) transaction.LocalOutcome {
	return b.state
}
func pendingAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	a, err := assessment.NewAssessment(7, testee.NewID(8), assessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"), assessment.NewAnswerSheetRef(meta.FromUint64(100)), assessment.NewAdhocOrigin(), assessment.WithID(meta.FromUint64(42)), assessment.WithCreatedAt(time.Unix(100, 0)), assessment.WithEvaluationModel(assessment.NewScaleEvaluationModelRef(meta.ZeroID, meta.NewCode("SCALE"), "1", "fixture")))
	require.NoError(t, err)
	c, err := assessment.NewConductingContext(10, time.Unix(90, 0), nil, 1, 1)
	require.NoError(t, err)
	assessment.WithConductingContext(c)(a)
	return a
}

type submissionWriteFacade struct {
	delegate *repoStub
	calls    int
}

func (f *submissionWriteFacade) SavePendingSubmission(ctx context.Context, a *assessment.Assessment) error {
	f.calls++
	return f.delegate.SavePendingSubmission(ctx, a)
}

func TestSubmitPreservesCASCallerAndFirstWire(t *testing.T) {
	b := &boundaryStub{t: t, state: transaction.LocalCommitted}
	repo := &repoStub{}
	facade := &submissionWriteFacade{delegate: repo}
	s := &Submitter{boundary: b, repo: repo, writer: facade, mapper: repository.NewAssessmentMapper()}
	pending := pendingAssessment(t)
	submitted, err := s.Submit(t.Context(), pending, time.Unix(200, 0))
	require.NoError(t, err)
	require.True(t, submitted.Status().IsSubmitted())
	require.True(t, pending.Status().IsPending())
	require.True(t, submitted.ConductingContext().Equal(pending.ConductingContext()))
	require.Equal(t, 1, repo.writes)
	require.Equal(t, 1, facade.calls, "original host write facade was bypassed")
	require.Empty(t, submitted.Events())
	wire := b.execution.Plan.Input().Branches[0].Wire
	repeated, err := s.Submit(t.Context(), pending, time.Unix(300, 0))
	require.NoError(t, err)
	require.Same(t, repo.stored, repeated)
	require.Equal(t, 1, b.executes)
	require.Equal(t, 1, b.reserves)
	require.Equal(t, wire, b.execution.Plan.Input().Branches[0].Wire)
	require.Equal(t, time.Unix(200, 0).UTC(), *repeated.SubmittedAt())
}
func TestSubmitUncertaintyDoesNotMutateOrRetry(t *testing.T) {
	for _, dbError := range []bool{false, true} {
		t.Run(map[bool]string{false: "half-unknown", true: "database-unavailable"}[dbError], func(t *testing.T) {
			b := &boundaryStub{t: t, state: transaction.LocalUnknown}
			if dbError {
				b.findErr = errors.New("primary unavailable")
			}
			repo := &repoStub{}
			s := &Submitter{boundary: b, repo: repo, writer: repo, mapper: repository.NewAssessmentMapper()}
			pending := pendingAssessment(t)
			got, err := s.Submit(t.Context(), pending, time.Unix(200, 0))
			require.ErrorIs(t, err, port.ErrUnresolved)
			require.Nil(t, got)
			require.True(t, pending.Status().IsPending())
			require.Zero(t, repo.writes)
			executes := b.executes
			_, err = s.Submit(t.Context(), pending, time.Unix(300, 0))
			require.ErrorIs(t, err, port.ErrUnresolved)
			require.Equal(t, executes, b.executes)
			if dbError {
				require.Zero(t, b.reserves)
			}
		})
	}
}
func TestSubmissionReadRejectsChangedBusinessIdentity(t *testing.T) {
	b := &boundaryStub{t: t, state: transaction.LocalCommitted}
	repo := &repoStub{}
	s := &Submitter{boundary: b, repo: repo, writer: repo, mapper: repository.NewAssessmentMapper()}
	pending := pendingAssessment(t)
	_, err := s.Submit(t.Context(), pending, time.Unix(200, 0))
	require.NoError(t, err)
	changed := repository.NewAssessmentMapper().ToPO(repo.stored)
	changed.AnswerSheetID++
	repo.stored = repository.NewAssessmentMapper().ToDomain(changed)
	_, err = s.Submit(t.Context(), pending, time.Unix(300, 0))
	require.ErrorIs(t, err, port.ErrUnresolved)
	require.Equal(t, 1, repo.writes)
}
