package commit

import (
	"context"
	"testing"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type commitTxMarker struct{}

type commitRunnerStub struct{}

func (commitRunnerStub) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, commitTxMarker{}, true))
}

type commitAssessmentRepoStub struct {
	order *[]string
}

func (r commitAssessmentRepoStub) Save(ctx context.Context, _ *assessment.Assessment) error {
	requireCommitTx(ctx)
	*r.order = append(*r.order, "assessment")
	return nil
}
func (commitAssessmentRepoStub) FindByID(context.Context, assessment.ID) (*assessment.Assessment, error) {
	return nil, nil
}
func (commitAssessmentRepoStub) FindByAnswerSheetID(context.Context, assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return nil, nil
}
func (commitAssessmentRepoStub) Delete(context.Context, assessment.ID) error { return nil }

type commitOutcomeRepoStub struct {
	order  *[]string
	record *domainoutcome.Record
}

func (r *commitOutcomeRepoStub) Save(ctx context.Context, record *domainoutcome.Record) error {
	requireCommitTx(ctx)
	*r.order = append(*r.order, "outcome")
	r.record = record
	return nil
}
func (*commitOutcomeRepoStub) FindByID(context.Context, domainoutcome.ID) (*domainoutcome.Record, error) {
	return nil, nil
}
func (*commitOutcomeRepoStub) FindByAssessmentID(context.Context, assessment.ID) (*domainoutcome.Record, error) {
	return nil, nil
}

type commitRunRepoStub struct {
	order *[]string
	saved *evalrun.EvaluationRun
}

func (r *commitRunRepoStub) Save(ctx context.Context, run evalrun.EvaluationRun) error {
	requireCommitTx(ctx)
	*r.order = append(*r.order, "run")
	r.saved = &run
	return nil
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
	order *[]string
}

func (p commitScoreProjectorStub) Project(ctx context.Context, _ evaloutcome.Outcome) error {
	requireCommitTx(ctx)
	*p.order = append(*p.order, "score")
	return nil
}

type commitEventStagerStub struct {
	order  *[]string
	events []event.DomainEvent
}

func (s *commitEventStagerStub) Stage(ctx context.Context, events ...event.DomainEvent) error {
	requireCommitTx(ctx)
	*s.order = append(*s.order, "event")
	s.events = append(s.events, events...)
	return nil
}

func TestCommitPersistsEvaluationFactsAndEventInOneTransaction(t *testing.T) {
	t.Parallel()

	a, execution := commitTestOutcome(t)
	order := make([]string, 0, 5)
	outcomeRepo := &commitOutcomeRepoStub{order: &order}
	runRepo := &commitRunRepoStub{order: &order}
	stager := &commitEventStagerStub{order: &order}
	c := NewCommitter(
		commitRunnerStub{},
		commitAssessmentRepoStub{order: &order},
		outcomeRepo,
		runRepo,
		commitScoreProjectorStub{order: &order},
		stager,
		nil,
	).(*committer)
	c.newID = func() meta.ID { return meta.FromUint64(9001) }
	run := evalrun.NewEvaluationRunWithAttempt(a.ID().Uint64(), 1)
	run.AttachInputSnapshot("model:SCALE-1@1.0.0")
	run.Start(time.Unix(100, 0))
	evaluatedAt := time.Unix(200, 0)

	record, err := c.Commit(context.Background(), Request{
		Outcome: evaloutcome.Outcome{
			Assessment:           a,
			Execution:            execution,
			RuntimeDescriptorKey: evalpipeline.RuntimeDescriptorKey{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring, DecisionKind: modelcatalog.DecisionKindScoreRange},
		},
		Run:         &run,
		EvaluatedAt: evaluatedAt,
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
	if !a.Status().IsEvaluated() || run.Attempt.Status != evalrun.StatusSucceeded || runRepo.saved == nil || runRepo.saved.Attempt.Status != evalrun.StatusSucceeded {
		t.Fatalf("terminal facts: assessment=%s run=%s saved=%#v", a.Status(), run.Attempt.Status, runRepo.saved)
	}
	if len(stager.events) != 1 {
		t.Fatalf("events = %d, want 1", len(stager.events))
	}
	evaluatedEvent, ok := stager.events[0].(assessment.AssessmentEvaluatedEvent)
	if !ok {
		t.Fatalf("event type = %T", stager.events[0])
	}
	payload := evaluatedEvent.Payload()
	if payload.OutcomeID != "9001" || payload.EvaluationRunID != run.RunID.String() {
		t.Fatalf("evaluated event payload = %#v", payload)
	}
	if len(a.Events()) != 0 {
		t.Fatalf("assessment events not cleared: %#v", a.Events())
	}
}

func commitTestOutcome(t *testing.T) (*assessment.Assessment, *assessment.AssessmentOutcome) {
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
	outcome := assessment.NewAssessmentOutcome(modelRef, assessment.ResultSummary{PrimaryLabel: "high"}, assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindScale})
	outcome.Primary = &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindRawTotal, Value: 12}
	outcome.Level = &assessment.OutcomeResultLevel{Code: "high", Label: "高风险", Severity: "high"}
	return a, outcome
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
