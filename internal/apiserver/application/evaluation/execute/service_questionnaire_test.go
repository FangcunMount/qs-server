package execute

import (
	"context"
	"errors"
	"testing"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type fakeAssessmentRepo struct {
	assessment         *domainAssessment.Assessment
	saveCalls          int
	saveCtxHadTxMarker bool
	saveErr            error
}

type engineTxCtxMarker struct{}

type engineRecordingTxRunner struct {
	called bool
	err    error
}

func (r *engineRecordingTxRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	r.called = true
	if r.err != nil {
		return r.err
	}
	return fn(context.WithValue(ctx, engineTxCtxMarker{}, true))
}

type engineRecordingEventStager struct {
	ctxHadTxMarker bool
	eventTypes     []string
	err            error
}

func (s *engineRecordingEventStager) Stage(ctx context.Context, events ...event.DomainEvent) error {
	s.ctxHadTxMarker, _ = ctx.Value(engineTxCtxMarker{}).(bool)
	for _, evt := range events {
		s.eventTypes = append(s.eventTypes, evt.EventType())
	}
	return s.err
}

func (r *fakeAssessmentRepo) Save(ctx context.Context, assessment *domainAssessment.Assessment) error {
	r.saveCtxHadTxMarker, _ = ctx.Value(engineTxCtxMarker{}).(bool)
	r.assessment = assessment
	r.saveCalls++
	return r.saveErr
}

func (r *fakeAssessmentRepo) FindByID(_ context.Context, _ domainAssessment.ID) (*domainAssessment.Assessment, error) {
	return r.assessment, nil
}

func (r *fakeAssessmentRepo) Delete(_ context.Context, _ domainAssessment.ID) error { return nil }
func (r *fakeAssessmentRepo) FindByAnswerSheetID(_ context.Context, _ domainAssessment.AnswerSheetRef) (*domainAssessment.Assessment, error) {
	return nil, nil
}

func TestEvaluateFailsWhenQuestionnaireVersionDoesNotResolveCurrentQuestionnaire(t *testing.T) {
	aRepo := &fakeAssessmentRepo{
		assessment: domainAssessment.Reconstruct(
			meta.FromUint64(101),
			1,
			testee.NewID(202),
			domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "0.9.0"),
			domainAssessment.NewAnswerSheetRef(meta.FromUint64(303)),
			domainAssessment.NewAdhocOrigin(),
			domainAssessment.StatusSubmitted,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			scaleModelRef(),
		),
	}

	svc := &service{
		assessmentRepo: aRepo,
		inputResolver:  failingInputResolver{err: inputFailure{reason: "加载问卷失败: 问卷不存在或版本不匹配"}},
		runRepo:        &stubRunRepo{},
		txRunner:       &engineRecordingTxRunner{},
		eventStager:    &engineRecordingEventStager{},
	}

	err := svc.Evaluate(context.Background(), 101)
	if err == nil {
		t.Fatal("Evaluate() error = nil, want questionnaire version failure")
	}
	if !aRepo.assessment.Status().IsFailed() {
		t.Fatalf("assessment status = %s, want failed", aRepo.assessment.Status())
	}
	if aRepo.saveCalls == 0 {
		t.Fatal("assessment should be persisted after markAsFailed")
	}
	if !aRepo.saveCtxHadTxMarker {
		t.Fatal("assessment Save should receive transaction context")
	}
}

func TestFailureFinalizerRequiresAtomicDependencies(t *testing.T) {
	a := engineAssessmentForOutboxTest(t)
	run := evalrun.NewEvaluationRunWithAttempt(a.ID().Uint64(), 1)
	if err := run.Start(time.Now()); err != nil {
		t.Fatal(err)
	}
	repo := &fakeAssessmentRepo{}
	finalizer := evaluationFailureFinalizer{repo: repo}

	err := finalizer.Finalize(context.Background(), a, &run, "pipeline failed", evalrun.Failure{Kind: evalrun.FailureKindInternal, Message: "pipeline failed"})
	if err == nil {
		t.Fatal("expected missing atomic failure dependencies to fail")
	}
	if repo.saveCalls != 0 {
		t.Fatalf("repository save calls = %d, want 0", repo.saveCalls)
	}
}

func TestMapInputResolveErrorPreservesExternalAPICodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		kind evaluationinput.FailureKind
		code int
	}{
		{name: "scale", kind: evaluationinput.FailureKindScaleNotFound, code: errorCode.ErrMedicalScaleNotFound},
		{name: "answer sheet", kind: evaluationinput.FailureKindAnswerSheetNotFound, code: errorCode.ErrAnswerSheetNotFound},
		{name: "questionnaire", kind: evaluationinput.FailureKindQuestionnaireNotFound, code: errorCode.ErrQuestionnaireNotFound},
		{name: "questionnaire version", kind: evaluationinput.FailureKindQuestionnaireVersionMismatch, code: errorCode.ErrQuestionnaireNotFound},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := evaluationinput.NewResolveError(tc.kind, errors.New("missing"), "missing", "load failed")
			mapped := mapInputResolveError(err)
			if !cberrors.IsCode(mapped, tc.code) {
				t.Fatalf("mapped code = %d, want %d", cberrors.ParseCoder(mapped).Code(), tc.code)
			}
			var reason evaluationinput.FailureReasonCarrier
			if !errors.As(mapped, &reason) || reason.FailureReason() != "load failed: missing" {
				t.Fatalf("mapped error should preserve failure reason, got %v", mapped)
			}
		})
	}
}

func TestFailureFinalizerStagesAssessmentRunAndOutboxThroughOneTransaction(t *testing.T) {
	a := engineAssessmentForOutboxTest(t)
	run := evalrun.NewEvaluationRunWithAttempt(a.ID().Uint64(), 1)
	if err := run.Start(time.Now()); err != nil {
		t.Fatal(err)
	}
	repo := &fakeAssessmentRepo{}
	runRepo := &stubRunRepo{}
	txRunner := &engineRecordingTxRunner{}
	stager := &engineRecordingEventStager{}
	finalizer := evaluationFailureFinalizer{repo: repo, runRepo: runRepo, txRunner: txRunner, eventStager: stager}

	if err := finalizer.Finalize(context.Background(), a, &run, "pipeline failed", evalrun.Failure{Kind: evalrun.FailureKindInternal, Message: "pipeline failed"}); err != nil {
		t.Fatalf("Finalize returned error: %v", err)
	}
	if !txRunner.called {
		t.Fatal("expected transaction runner to be used")
	}
	if repo.saveCalls != 1 {
		t.Fatalf("repository save calls = %d, want 1", repo.saveCalls)
	}
	if !repo.saveCtxHadTxMarker {
		t.Fatal("assessment Save should receive transaction context")
	}
	if len(runRepo.saved) != 1 || runRepo.saved[0].Attempt.Status != evalrun.StatusFailed {
		t.Fatalf("saved runs = %#v, want one failed run", runRepo.saved)
	}
	if !runRepo.saveCtxHadTxMarker {
		t.Fatal("evaluation run Save should receive transaction context")
	}
	if !stager.ctxHadTxMarker {
		t.Fatal("event stager should receive transaction context")
	}
	if len(stager.eventTypes) != 1 || stager.eventTypes[0] != domainAssessment.EventTypeFailed {
		t.Fatalf("staged event types = %#v, want assessment failed", stager.eventTypes)
	}
	if len(a.Events()) != 0 {
		t.Fatalf("expected events to be cleared after successful transaction, got %d", len(a.Events()))
	}
}

func TestFailureFinalizerTransactionErrorsDoNotPolluteCallerState(t *testing.T) {
	t.Parallel()

	commitErr := errors.New("failure transaction node failed")
	cases := []struct {
		name      string
		configure func(*fakeAssessmentRepo, *stubRunRepo, *engineRecordingTxRunner, *engineRecordingEventStager)
	}{
		{
			name: "transaction runner",
			configure: func(_ *fakeAssessmentRepo, _ *stubRunRepo, runner *engineRecordingTxRunner, _ *engineRecordingEventStager) {
				runner.err = commitErr
			},
		},
		{
			name: "assessment",
			configure: func(repo *fakeAssessmentRepo, _ *stubRunRepo, _ *engineRecordingTxRunner, _ *engineRecordingEventStager) {
				repo.saveErr = commitErr
			},
		},
		{
			name: "run",
			configure: func(_ *fakeAssessmentRepo, repo *stubRunRepo, _ *engineRecordingTxRunner, _ *engineRecordingEventStager) {
				repo.saveErr = commitErr
			},
		},
		{
			name: "outbox",
			configure: func(_ *fakeAssessmentRepo, _ *stubRunRepo, _ *engineRecordingTxRunner, stager *engineRecordingEventStager) {
				stager.err = commitErr
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := engineAssessmentForOutboxTest(t)
			a.ClearEvents()
			run := evalrun.NewEvaluationRunWithAttempt(a.ID().Uint64(), 1)
			if err := run.Start(time.Unix(100, 0)); err != nil {
				t.Fatal(err)
			}
			repo := &fakeAssessmentRepo{assessment: a}
			runRepo := &stubRunRepo{}
			runner := &engineRecordingTxRunner{}
			stager := &engineRecordingEventStager{}
			tc.configure(repo, runRepo, runner, stager)
			finalizer := evaluationFailureFinalizer{repo: repo, runRepo: runRepo, txRunner: runner, eventStager: stager}

			err := finalizer.Finalize(context.Background(), a, &run, "pipeline failed", evalrun.Failure{Kind: evalrun.FailureKindInternal, Message: "pipeline failed"})
			if !errors.Is(err, commitErr) {
				t.Fatalf("Finalize() error = %v, want %v", err, commitErr)
			}
			if !a.Status().IsSubmitted() || a.FailedAt() != nil || a.FailureReason() != nil || len(a.Events()) != 0 {
				t.Fatalf("caller Assessment polluted: status=%s failed_at=%v reason=%v events=%v", a.Status(), a.FailedAt(), a.FailureReason(), a.Events())
			}
			if run.Attempt.Status != evalrun.StatusRunning || run.FinishedAt != nil || run.Failure != nil {
				t.Fatalf("caller Run polluted: %#v", run)
			}
		})
	}
}

func engineAssessmentForOutboxTest(t *testing.T) *domainAssessment.Assessment {
	t.Helper()
	return domainAssessment.Reconstruct(
		meta.FromUint64(9901),
		1,
		testee.NewID(202),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "0.9.0"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(303)),
		domainAssessment.NewAdhocOrigin(),
		domainAssessment.StatusSubmitted,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		scaleModelRef(),
	)
}

type failingInputResolver struct {
	err error
}

func (r failingInputResolver) Resolve(context.Context, evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	return nil, r.err
}

type successfulInputResolver struct {
	snapshot *evaluationinput.InputSnapshot
	lastRef  evaluationinput.InputRef
	calls    int
}

func (r *successfulInputResolver) Resolve(_ context.Context, ref evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	r.calls++
	r.lastRef = ref
	return r.snapshot, nil
}

type inputFailure struct {
	reason string
}

func (e inputFailure) Error() string {
	return e.reason
}

func (e inputFailure) FailureReason() string {
	return e.reason
}

func TestEvaluateDispatchesScaleModelToScaleEvaluator(t *testing.T) {
	aRepo := &fakeAssessmentRepo{
		assessment: domainAssessment.Reconstruct(
			meta.FromUint64(101),
			1,
			testee.NewID(202),
			domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
			domainAssessment.NewAnswerSheetRef(meta.FromUint64(303)),
			domainAssessment.NewAdhocOrigin(),
			domainAssessment.StatusSubmitted,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			scaleModelRef(),
		),
	}
	input := &successfulInputResolver{snapshot: &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:  evaluationinput.EvaluationModelKindScale,
			Code:  "S-001",
			Title: "Scale",
		},
		ModelPayload:  evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{Code: "S-001", Title: "Scale"}},
		AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: 303, QuestionnaireCode: "Q-001", QuestionnaireVersion: "1.0.0"},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-001", Version: "1.0.0"},
	}}
	capture := &splitPhaseCapture{}
	var executionInput ExecutionInput
	evaluator := evaluatorStub{
		key: evaluation.ExecutionIdentityScaleDefault,
		execute: func(ctx context.Context, input ExecutionInput) (*domainoutcome.Execution, error) {
			executionInput = input
			modelRef := *input.Assessment.EvaluationModelRef()
			score := 7.0
			level := string(domainAssessment.RiskLevelLow)
			execution := domainoutcome.NewExecution(
				evaloutcome.ModelRefFromAssessment(modelRef),
				domainoutcome.Summary{
					PrimaryLabel: "ok",
					Score:        &score,
					Level:        &level,
				},
				domainoutcome.Detail{Kind: modelRef.Kind()},
			)
			execution.Primary = &domainoutcome.ScoreValue{
				Kind:  domainoutcome.ScoreKindRawTotal,
				Value: score,
			}
			execution.Level = &domainoutcome.ResultLevel{Code: level, Label: "ok"}
			return execution, nil
		},
	}
	svc := newSplitPhaseTestService(aRepo, input, capture, withTestEvaluator(evaluator))

	if err := svc.Evaluate(context.Background(), 101); err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if executionInput.Assessment != aRepo.assessment || executionInput.Input != input.snapshot {
		t.Fatalf("unexpected executor input: %#v", executionInput)
	}
	if capture.CommitCalls != 1 || capture.Request.Execution == nil || capture.Request.Execution.Primary == nil || capture.Request.Execution.Primary.Value != 7 {
		t.Fatalf("unexpected evaluation execution: %#v", capture.Request.Execution)
	}
	if input.calls != 1 || input.lastRef.ModelRef.Kind != evaluationinput.EvaluationModelKindScale || input.lastRef.ModelRef.Code != "S-001" {
		t.Fatalf("unexpected input ref: %#v", input.lastRef)
	}
}

func scaleModelRef() *domainAssessment.EvaluationModelRef {
	ref := domainAssessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("S-001"), "1.0.0", "Scale")
	return &ref
}

func TestEvaluateDispatchesNonScaleModelThroughRegistry(t *testing.T) {
	modelRef := domainAssessment.NewEvaluationModelRefWithIdentity(
		domainAssessment.EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmMBTI,
		meta.ID(0),
		meta.NewCode("FAKE-MODEL"),
		"1.0.0",
		"Fake Model",
	)
	aRepo := &fakeAssessmentRepo{
		assessment: domainAssessment.Reconstruct(
			meta.FromUint64(103),
			1,
			testee.NewID(202),
			domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-FAKE"), "1.0.0"),
			domainAssessment.NewAnswerSheetRef(meta.FromUint64(305)),
			domainAssessment.NewAdhocOrigin(),
			domainAssessment.StatusSubmitted,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			&modelRef,
		),
	}
	input := &successfulInputResolver{snapshot: &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:      evaluationinput.EvaluationModelKindPersonality,
			SubKind:   string(modelcatalog.SubKindTypology),
			Algorithm: string(modelcatalog.AlgorithmMBTI),
			Code:      "FAKE-MODEL",
			Version:   "1.0.0",
			Title:     "Fake Model",
		},
		AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: 305, QuestionnaireCode: "Q-FAKE", QuestionnaireVersion: "1.0.0"},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-FAKE", Version: "1.0.0"},
	}}
	capture := &splitPhaseCapture{}
	evaluator := evaluatorStub{
		key: evaluation.PersonalityTypologyIdentity(modelcatalog.AlgorithmMBTI),
		execute: func(ctx context.Context, input ExecutionInput) (*domainoutcome.Execution, error) {
			modelRef := *input.Assessment.EvaluationModelRef()
			execution := domainoutcome.NewExecution(
				evaloutcome.ModelRefFromAssessment(modelRef),
				domainoutcome.Summary{PrimaryLabel: "INTJ"},
				domainoutcome.Detail{
					Kind:    modelRef.Kind(),
					Payload: "INTJ",
				},
			)
			execution.Level = &domainoutcome.ResultLevel{
				Code:     "INTJ",
				Label:    "INTJ",
				Severity: "none",
			}
			return execution, nil
		},
	}
	svc := newSplitPhaseTestService(aRepo, input, capture, withTestEvaluator(evaluator))

	if err := svc.Evaluate(context.Background(), 103); err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if capture.CommitCalls != 1 || capture.Request.Execution == nil || capture.Request.Execution.ModelRef.Kind() != modelcatalog.KindTypology {
		t.Fatalf("unexpected evaluation execution: %#v", capture.Request.Execution)
	}
	if input.lastRef.ModelRef.Kind != evaluationinput.EvaluationModelKindPersonality || input.lastRef.ModelRef.Code != "FAKE-MODEL" {
		t.Fatalf("unexpected input ref: %#v", input.lastRef)
	}
}

func TestEvaluateUnknownRuleSetKindMarksAssessmentFailed(t *testing.T) {
	modelRef := domainAssessment.NewEvaluationModelRefByCode(domainAssessment.EvaluationModelKindPersonality, meta.NewCode("MBTI-16P"), "1.0.0", "MBTI")
	aRepo := &fakeAssessmentRepo{
		assessment: domainAssessment.Reconstruct(
			meta.FromUint64(102),
			1,
			testee.NewID(202),
			domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-MBTI"), "1.0.0"),
			domainAssessment.NewAnswerSheetRef(meta.FromUint64(304)),
			domainAssessment.NewAdhocOrigin(),
			domainAssessment.StatusSubmitted,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			&modelRef,
		),
	}
	input := &successfulInputResolver{snapshot: &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:    evaluationinput.EvaluationModelKindPersonality,
			Code:    "MBTI-16P",
			Version: "1.0.0",
			Title:   "MBTI",
		},
		AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: 304, QuestionnaireCode: "Q-MBTI", QuestionnaireVersion: "1.0.0"},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-MBTI", Version: "1.0.0"},
	}}
	capture := &splitPhaseCapture{}
	txRunner := &engineRecordingTxRunner{}
	stager := &engineRecordingEventStager{}
	evaluator := evaluatorStub{key: evaluation.ExecutionIdentityScaleDefault}
	svc := newSplitPhaseTestService(
		aRepo,
		input,
		capture,
		WithTransactionalOutbox(txRunner, stager),
		withTestEvaluator(evaluator),
	)

	err := svc.Evaluate(context.Background(), 102)
	if err == nil {
		t.Fatal("Evaluate error = nil, want unsupported model kind")
	}
	if !aRepo.assessment.Status().IsFailed() {
		t.Fatalf("assessment status = %s, want failed", aRepo.assessment.Status())
	}
	if aRepo.saveCalls != 1 || !txRunner.called {
		t.Fatalf("only terminal Assessment failure must persist transactionally: saveCalls=%d tx=%v", aRepo.saveCalls, txRunner.called)
	}
	if len(stager.eventTypes) != 1 || stager.eventTypes[0] != domainAssessment.EventTypeFailed {
		t.Fatalf("staged event types = %#v, want assessment failed", stager.eventTypes)
	}
}
