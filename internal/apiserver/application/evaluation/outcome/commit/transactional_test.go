package commit

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
)

type transactionResolver struct{}

func (transactionResolver) GetTopicForEvent(string) (string, bool) { return "evaluation", true }

type outcomeBoundaryStub struct {
	t           *testing.T
	order       *[]string
	reserveErr  error
	acquired    bool
	local       transaction.LocalOutcome
	commitErr   error
	executes    int
	request     port.Request
	beforeWrite func()
	execution   transaction.Execution
	findErr     error
}

func (s *outcomeBoundaryStub) Find(context.Context, string, string) (transaction.Reservation, error) {
	if s.findErr != nil {
		return transaction.Reservation{}, s.findErr
	}
	if !s.execution.Plan.Valid() {
		return transaction.Reservation{}, transaction.ErrNotFound
	}
	return transaction.Reservation{Execution: s.execution}, nil
}

func (s *outcomeBoundaryStub) Reserve(_ context.Context, q port.Request) (transaction.Reservation, error) {
	s.request = q
	if s.reserveErr != nil {
		return transaction.Reservation{}, s.reserveErr
	}
	*s.order = append(*s.order, "reserve")
	intents, err := standard.PrepareIntents(q.Events, transactionResolver{}, "api-server")
	require.NoError(s.t, err)
	var branches []transaction.BranchInput
	for _, intent := range intents {
		in := intent.Message.Input()
		wire := bytes.Clone(in.Payload)
		in.Payload = bytes.Clone(q.Artifact)
		m, err := message.New(in)
		require.NoError(s.t, err)
		physical, err := transport.PhysicalTopicV2(in.Destination, transport.TransactionMessageType, "qs.mysql", "m3-1")
		require.NoError(s.t, err)
		branches = append(branches, transaction.BranchInput{ID: in.ID, Message: m, Wire: wire, PhysicalTopic: physical, RouteRevision: "m3-1"})
	}
	p, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mysql", Scope: intents[0].Message.Input().Scope, OperationID: q.Events[0].EventID(), IdempotencyKey: q.Key, InputHash: q.InputHash, Branches: branches})
	require.NoError(s.t, err)
	s.execution = transaction.Execution{Plan: p, Fence: 1, WriterEpoch: "m3-1", Token: [32]byte{1}, LeaseUntil: time.Now().Add(time.Minute)}
	return transaction.Reservation{Acquired: s.acquired, State: transaction.Prepared, Execution: s.execution}, nil
}
func (s *outcomeBoundaryStub) Artifact(p transaction.FrozenPlan) ([]byte, error) {
	return p.Input().Branches[0].Message.Input().Payload, nil
}
func (s *outcomeBoundaryStub) Commit(ctx context.Context, _ transaction.Execution, write func(context.Context) error) (transaction.Result, error) {
	s.executes++
	*s.order = append(*s.order, "prepared-all")
	if s.beforeWrite != nil {
		s.beforeWrite()
	}
	if s.local == transaction.LocalCommitted {
		*s.order = append(*s.order, "guard")
		if err := write(context.WithValue(ctx, commitTxMarker{}, true)); err != nil {
			return transaction.Result{Local: transaction.LocalUnknown}, err
		}
		*s.order = append(*s.order, "witness-root-commit")
	}
	return transaction.Result{Local: s.local, Branches: []transaction.BranchResult{{Half: transaction.HalfPrepared, Final: transaction.FinalUnknown}}}, s.commitErr
}
func (s *outcomeBoundaryStub) Confirm(context.Context, transaction.Execution) transaction.LocalOutcome {
	return s.local
}

func transactionCommitFixture(t *testing.T, boundary *outcomeBoundaryStub) (*committer, CommitRequest, *commitOutcomeRepoStub) {
	t.Helper()
	a, execution := commitTestOutcome(t)
	run := evalrun.NewEvaluationRunWithAttempt(a.ID().Uint64(), 1)
	require.NoError(t, run.Start(time.Unix(100, 0)))
	require.NoError(t, run.AttachInputSnapshot("isn:v2:"+strings.Repeat("a", 64)))
	repo := &commitOutcomeRepoStub{order: boundary.order}
	c, err := NewTransactionalCommitter(boundary, commitAssessmentRepoStub{order: boundary.order}, repo, &commitRunRepoStub{order: boundary.order}, commitScoreProjectorStub{order: boundary.order})
	require.NoError(t, err)
	return c.(*committer), CommitRequest{Assessment: a, Input: commitTestInput(), Execution: execution, Run: &run, EvaluatedAt: time.Unix(200, 0), DescriptorKey: evalpipeline.DescriptorKey{DecisionKind: modelcatalog.DecisionKindScoreRange}, OutcomePolicy: evalpipeline.DefaultOutcomeCompletenessPolicy(modelcatalog.DecisionKindScoreRange)}, repo
}

func TestTransactionOutcomeCommitsOriginalFactsWithoutOutbox(t *testing.T) {
	var order []string
	s := &outcomeBoundaryStub{t: t, order: &order, acquired: true, local: transaction.LocalCommitted, commitErr: errors.New("final confirmation response lost")}
	c, q, repo := transactionCommitFixture(t, s)
	s.beforeWrite = func() {
		require.False(t, q.Assessment.Status().IsEvaluated())
		require.Equal(t, evalrun.StatusRunning, q.Run.Attempt().Status)
	}
	record, err := c.Commit(t.Context(), q)
	require.NoError(t, err, "known local commit remains successful even when final confirmation is unknown")
	require.Equal(t, []string{"reserve", "prepared-all", "guard", "outcome", "score", "assessment", "run", "witness-root-commit"}, order)
	require.Same(t, record, repo.record)
	require.Nil(t, record.CommittedEventEvidence())
	require.NotNil(t, record.CommittedTransactionEvidence())
	require.True(t, q.Assessment.Status().IsEvaluated())
	require.Equal(t, evalrun.StatusSucceeded, q.Run.Attempt().Status)
	require.Empty(t, q.Assessment.Events())
	input, err := readOutcomeArtifact(s.request.Artifact)
	require.NoError(t, err)
	require.Equal(t, record.BusinessBindingSHA256(), domainoutcome.BusinessBindingSHA256(input))
}

func TestTransactionOutcomeUnknownAndLoserNeverPublishCallerState(t *testing.T) {
	for _, tc := range []struct {
		name       string
		acquired   bool
		reserveErr error
	}{
		{"half-unknown", true, nil}, {"concurrent-loser", false, nil}, {"admission-closed", true, errors.New("closed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var order []string
			s := &outcomeBoundaryStub{t: t, order: &order, acquired: tc.acquired, local: transaction.LocalUnknown, reserveErr: tc.reserveErr}
			c, q, repo := transactionCommitFixture(t, s)
			record, err := c.Commit(t.Context(), q)
			require.ErrorIs(t, err, port.ErrUnresolved)
			require.Nil(t, record)
			require.Nil(t, repo.record)
			require.False(t, q.Assessment.Status().IsEvaluated())
			require.Equal(t, evalrun.StatusRunning, q.Run.Attempt().Status)
			if !tc.acquired || tc.reserveErr != nil {
				require.Zero(t, s.executes)
			}
		})
	}
}

func TestOutcomeArtifactKeepsOriginalBytesAndStableInput(t *testing.T) {
	var order []string
	s := &outcomeBoundaryStub{t: t, order: &order, acquired: true, local: transaction.LocalCommitted}
	c, q, _ := transactionCommitFixture(t, s)
	_, err := c.Commit(t.Context(), q)
	require.NoError(t, err)
	in, err := readOutcomeArtifact(s.request.Artifact)
	require.NoError(t, err)
	in.Payload = []byte("{ \"html\" : \"<>&\", \"n\": 1.00 }\n")
	in.ReportInput = []byte("{\n  \"raw\":true\n}\n")
	a, err := outcomeRequest(in, []event.DomainEvent{})
	require.NoError(t, err)
	roundtrip, err := readOutcomeArtifact(a.Artifact)
	require.NoError(t, err)
	require.Equal(t, []byte(in.Payload), []byte(roundtrip.Payload))
	require.Equal(t, []byte(in.ReportInput), []byte(roundtrip.ReportInput))
	in.ID, in.EvaluatedAt = meta.New(), in.EvaluatedAt.Add(time.Minute)
	b, err := outcomeRequest(in, nil)
	require.NoError(t, err)
	require.Equal(t, a.InputHash, b.InputHash)
	require.NotEqual(t, a.Artifact, b.Artifact)
	in.Payload = []byte(`{"changed":true}`)
	different, err := outcomeRequest(in, nil)
	require.NoError(t, err)
	require.NotEqual(t, a.InputHash, different.InputHash)
}

type recoveryRunRepo struct {
	*commitRunRepoStub
	latest *evalrun.EvaluationRun
}

func (r recoveryRunRepo) FindLatestByAssessmentID(context.Context, uint64) (*evalrun.EvaluationRun, error) {
	return r.latest, nil
}

type recoveryOutcomeRepo struct{ *commitOutcomeRepoStub }

func (r recoveryOutcomeRepo) FindByID(context.Context, domainoutcome.ID) (*domainoutcome.Record, error) {
	return r.record, nil
}

type recoveryAssessmentRepo struct {
	commitAssessmentRepoStub
	stored *assessment.Assessment
}

func (r recoveryAssessmentRepo) FindByID(context.Context, assessment.ID) (*assessment.Assessment, error) {
	return r.stored, nil
}

func TestPendingOutcomeRecoveryNeverReexecutesBusiness(t *testing.T) {
	var order []string
	boundary := &outcomeBoundaryStub{t: t, order: &order, acquired: true, local: transaction.LocalCommitted}
	c, q, repo := transactionCommitFixture(t, boundary)
	stale := *q.Assessment
	record, err := c.Commit(t.Context(), q)
	require.NoError(t, err)
	require.NotNil(t, record)
	recovered, err := NewTransactionalCommitter(boundary, recoveryAssessmentRepo{commitAssessmentRepoStub{order: &order}, q.Assessment}, recoveryOutcomeRepo{repo}, recoveryRunRepo{&commitRunRepoStub{order: &order}, q.Run}, nil)
	require.NoError(t, err)
	checker := recovered.(PendingRecoverer)
	for _, tc := range []struct {
		name       string
		state      transaction.LocalOutcome
		findErr    error
		successful bool
	}{
		{"prepared-blocks-expired-lease", transaction.LocalUnknown, nil, false},
		{"aborted-requires-explicit-host-decision", transaction.LocalAborted, nil, false},
		{"database-error-is-not-not-found", transaction.LocalCommitted, errors.New("database unavailable"), false},
		{"committed-recovers-original-result", transaction.LocalCommitted, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			boundary.local, boundary.findErr = tc.state, tc.findErr
			candidate := stale
			before := len(order)
			handled, err := checker.RecoverPending(t.Context(), &candidate)
			require.True(t, handled)
			if tc.successful {
				require.NoError(t, err)
				require.True(t, candidate.Status().IsEvaluated())
			} else {
				require.Error(t, err)
				require.True(t, candidate.Status().IsSubmitted())
			}
			require.Equal(t, before, len(order), "recovery wrote business facts or sent messages")
			require.Equal(t, 1, boundary.executes)
		})
	}
}
