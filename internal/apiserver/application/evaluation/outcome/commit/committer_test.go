package commit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type commitTxMarker struct{}

type commitRunnerStub struct{}

func (commitRunnerStub) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, commitTxMarker{}, true))
}

type commitAssessmentRepoStub struct {
	order   *[]string
	saveErr error
}

func (r commitAssessmentRepoStub) Save(ctx context.Context, _ *assessment.Assessment) error {
	requireCommitTx(ctx)
	*r.order = append(*r.order, "assessment")
	return r.saveErr
}
func (commitAssessmentRepoStub) FindByID(context.Context, assessment.ID) (*assessment.Assessment, error) {
	return nil, nil
}
func (commitAssessmentRepoStub) FindByAnswerSheetID(context.Context, assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return nil, nil
}
func (commitAssessmentRepoStub) Delete(context.Context, assessment.ID) error { return nil }

type commitOutcomeRepoStub struct {
	order   *[]string
	record  *domainoutcome.Record
	saveErr error
}

func (r *commitOutcomeRepoStub) Save(ctx context.Context, record *domainoutcome.Record) error {
	requireCommitTx(ctx)
	*r.order = append(*r.order, "outcome")
	r.record = record
	return r.saveErr
}
func (*commitOutcomeRepoStub) FindByID(context.Context, domainoutcome.ID) (*domainoutcome.Record, error) {
	return nil, nil
}
func (*commitOutcomeRepoStub) FindByAssessmentID(context.Context, assessment.ID) (*domainoutcome.Record, error) {
	return nil, nil
}

type commitRunRepoStub struct {
	order   *[]string
	saved   *evalrun.EvaluationRun
	saveErr error
}

func (*commitRunRepoStub) Claim(context.Context, evaluationrun.ClaimRequest) (evaluationrun.ClaimResult, error) {
	return evaluationrun.ClaimResult{}, nil
}
func (r *commitRunRepoStub) SaveClaimed(ctx context.Context, run evalrun.EvaluationRun) error {
	requireCommitTx(ctx)
	*r.order = append(*r.order, "run")
	r.saved = &run
	return r.saveErr
}
func (*commitRunRepoStub) FindLatestByAssessmentID(context.Context, uint64) (*evalrun.EvaluationRun, error) {
	return nil, nil
}
func (*commitRunRepoStub) ListByAssessmentID(context.Context, uint64, int) ([]evalrun.EvaluationRun, error) {
	return nil, nil
}
func (*commitRunRepoStub) ListRetryableFailed(context.Context, evaluationrun.ListRetryableFailedParams) (*evaluationrun.ListRetryableFailedResult, error) {
	return &evaluationrun.ListRetryableFailedResult{}, nil
}

type commitScoreProjectorStub struct {
	order      *[]string
	projectErr error
}

func (p commitScoreProjectorStub) Project(ctx context.Context, _ *domainoutcome.Record, _ *assessment.Assessment, _ *domainoutcome.Execution) error {
	requireCommitTx(ctx)
	*p.order = append(*p.order, "score")
	return p.projectErr
}

type commitEventStagerStub struct {
	order    *[]string
	events   []event.DomainEvent
	stageErr error
}

type commitPostCommitStub struct {
	calls  int
	events []event.DomainEvent
	at     time.Time
}

func (s *commitPostCommitStub) AfterCommit(_ context.Context, events []event.DomainEvent, at time.Time) {
	s.calls++
	s.events = append(s.events, events...)
	s.at = at
}

func (s *commitEventStagerStub) Stage(ctx context.Context, events ...event.DomainEvent) error {
	requireCommitTx(ctx)
	*s.order = append(*s.order, "event")
	s.events = append(s.events, events...)
	return s.stageErr
}

func TestCommitPersistsEvaluationFactsAndEventInOneTransaction(t *testing.T) {
	t.Parallel()

	a, execution := commitTestOutcome(t)
	order := make([]string, 0, 5)
	outcomeRepo := &commitOutcomeRepoStub{order: &order}
	runRepo := &commitRunRepoStub{order: &order}
	stager := &commitEventStagerStub{order: &order}
	postCommit := &commitPostCommitStub{}
	c := NewCommitter(
		commitRunnerStub{},
		commitAssessmentRepoStub{order: &order},
		outcomeRepo,
		runRepo,
		commitScoreProjectorStub{order: &order},
		stager,
		postCommit,
	).(*committer)
	c.newID = func() meta.ID { return meta.FromUint64(9001) }
	run := evalrun.NewEvaluationRunWithAttempt(a.ID().Uint64(), 1)
	if err := run.Start(time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	if err := run.AttachInputSnapshot("model:SCALE-1@1.0.0"); err != nil {
		t.Fatal(err)
	}
	evaluatedAt := time.Unix(200, 0)

	record, err := c.Commit(context.Background(), CommitRequest{
		Assessment:    a,
		Execution:     execution,
		DescriptorKey: evalpipeline.DescriptorKey{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring, DecisionKind: modelcatalog.DecisionKindScoreRange},
		Run:           &run,
		EvaluatedAt:   evaluatedAt,
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if got, want := joinOrder(order), "outcome,score,assessment,run,event"; got != want {
		t.Fatalf("commit order = %s, want %s", got, want)
	}
	if record.ID().String() != "9001" || outcomeRepo.record != record {
		t.Fatalf("outcome record = %#v", record)
	}
	if !a.Status().IsEvaluated() || run.Attempt().Status != evalrun.StatusSucceeded || runRepo.saved == nil || runRepo.saved.Attempt().Status != evalrun.StatusSucceeded {
		t.Fatalf("terminal facts: assessment=%s run=%s saved=%#v", a.Status(), run.Attempt().Status, runRepo.saved)
	}
	if a.EvaluatedAt() == nil || !a.EvaluatedAt().Equal(evaluatedAt) {
		t.Fatalf("assessment evaluated_at = %v, want %v", a.EvaluatedAt(), evaluatedAt)
	}
	if run.FinishedAt() == nil || !run.FinishedAt().Equal(evaluatedAt) || !record.EvaluatedAt().Equal(evaluatedAt) {
		t.Fatalf("terminal timestamps: assessment=%v run=%v outcome=%v, want %v", a.EvaluatedAt(), run.FinishedAt(), record.EvaluatedAt(), evaluatedAt)
	}
	if len(stager.events) != 1 {
		t.Fatalf("events = %d, want 1", len(stager.events))
	}
	if postCommit.calls != 1 || len(postCommit.events) != 1 || postCommit.events[0].EventType() != "evaluation.outcome.committed" || !postCommit.at.Equal(evaluatedAt) {
		t.Fatalf("post-commit = calls:%d events:%v at:%v", postCommit.calls, postCommit.events, postCommit.at)
	}
	evaluatedEvent, ok := stager.events[0].(assessment.EvaluationOutcomeCommittedEvent)
	if !ok {
		t.Fatalf("event type = %T", stager.events[0])
	}
	payload := evaluatedEvent.Payload()
	if payload.OutcomeID != "9001" || payload.EvaluationRunID != run.ID().String() {
		t.Fatalf("evaluated event payload = %#v", payload)
	}
	if !payload.CommittedAt.Equal(evaluatedAt) {
		t.Fatalf("event committed_at = %v, want %v", payload.CommittedAt, evaluatedAt)
	}
	if len(a.Events()) != 0 {
		t.Fatalf("assessment events not cleared: %#v", a.Events())
	}
}

func TestCommitFailureDoesNotPublishPreparedTerminalStateToCaller(t *testing.T) {
	t.Parallel()

	commitErr := errors.New("commit node failed")
	cases := []struct {
		name      string
		configure func(*commitAssessmentRepoStub, *commitOutcomeRepoStub, *commitRunRepoStub, *commitScoreProjectorStub, *commitEventStagerStub)
	}{
		{
			name: "outcome",
			configure: func(_ *commitAssessmentRepoStub, repo *commitOutcomeRepoStub, _ *commitRunRepoStub, _ *commitScoreProjectorStub, _ *commitEventStagerStub) {
				repo.saveErr = commitErr
			},
		},
		{
			name: "score projection",
			configure: func(_ *commitAssessmentRepoStub, _ *commitOutcomeRepoStub, _ *commitRunRepoStub, projector *commitScoreProjectorStub, _ *commitEventStagerStub) {
				projector.projectErr = commitErr
			},
		},
		{
			name: "assessment",
			configure: func(repo *commitAssessmentRepoStub, _ *commitOutcomeRepoStub, _ *commitRunRepoStub, _ *commitScoreProjectorStub, _ *commitEventStagerStub) {
				repo.saveErr = commitErr
			},
		},
		{
			name: "run",
			configure: func(_ *commitAssessmentRepoStub, _ *commitOutcomeRepoStub, repo *commitRunRepoStub, _ *commitScoreProjectorStub, _ *commitEventStagerStub) {
				repo.saveErr = commitErr
			},
		},
		{
			name: "outbox",
			configure: func(_ *commitAssessmentRepoStub, _ *commitOutcomeRepoStub, _ *commitRunRepoStub, _ *commitScoreProjectorStub, stager *commitEventStagerStub) {
				stager.stageErr = commitErr
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a, execution := commitTestOutcome(t)
			order := make([]string, 0, 5)
			assessmentRepo := &commitAssessmentRepoStub{order: &order}
			outcomeRepo := &commitOutcomeRepoStub{order: &order}
			runRepo := &commitRunRepoStub{order: &order}
			projector := &commitScoreProjectorStub{order: &order}
			stager := &commitEventStagerStub{order: &order}
			tc.configure(assessmentRepo, outcomeRepo, runRepo, projector, stager)
			c := NewCommitter(commitRunnerStub{}, assessmentRepo, outcomeRepo, runRepo, projector, stager, nil)
			run := evalrun.NewEvaluationRunWithAttempt(a.ID().Uint64(), 1)
			if err := run.Start(time.Unix(100, 0)); err != nil {
				t.Fatal(err)
			}

			_, err := c.Commit(context.Background(), CommitRequest{
				Assessment:    a,
				Execution:     execution,
				DescriptorKey: evalpipeline.DescriptorKey{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring, DecisionKind: modelcatalog.DecisionKindScoreRange},
				Run:           &run,
				EvaluatedAt:   time.Unix(200, 0),
			})
			if !errors.Is(err, commitErr) {
				t.Fatalf("Commit error = %v, want %v", err, commitErr)
			}
			if !a.Status().IsSubmitted() || a.EvaluatedAt() != nil || a.TotalScore() != nil {
				t.Fatalf("caller assessment was polluted: status=%s evaluated_at=%v total_score=%v", a.Status(), a.EvaluatedAt(), a.TotalScore())
			}
			if run.Attempt().Status != evalrun.StatusRunning || run.FinishedAt() != nil || run.Failure() != nil {
				t.Fatalf("caller run was polluted: %#v", run)
			}
			if len(a.Events()) != 0 {
				t.Fatalf("caller assessment events were polluted: %#v", a.Events())
			}
		})
	}
}

func commitTestOutcome(t *testing.T) (*assessment.Assessment, *domainoutcome.Execution) {
	t.Helper()
	modelRef := assessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("SCALE-1"), "1.0.0", "scale")
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(1001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(2001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(5001)),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Submit(); err != nil {
		t.Fatal(err)
	}
	a.ClearEvents()
	execution := domainoutcome.NewExecution(evaloutcome.ModelRefFromAssessment(modelRef), domainoutcome.Summary{PrimaryLabel: "high"}, domainoutcome.Detail{Kind: modelcatalog.KindScale})
	execution.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 12}
	execution.Level = &domainoutcome.ResultLevel{Code: "high", Label: "高风险", Severity: "high"}
	return a, execution
}

func requireCommitTx(ctx context.Context) {
	if ok, _ := ctx.Value(commitTxMarker{}).(bool); !ok {
		panic("commit dependency called outside transaction context")
	}
}

func joinOrder(items []string) string {
	var result string
	for i, item := range items {
		if i > 0 {
			result += ","
		}
		result += item
	}
	return result
}
