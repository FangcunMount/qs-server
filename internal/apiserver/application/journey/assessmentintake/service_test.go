package assessmentintake

import (
	"context"
	"errors"
	"reflect"
	"testing"

	evaluationintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type scoringStub struct {
	calls *[]string
	err   error
}

func (s scoringStub) CalculateAndSave(context.Context, uint64) error {
	*s.calls = append(*s.calls, "score")
	return s.err
}

type intakeStub struct {
	calls     *[]string
	created   *evaluationintake.Assessment
	existing  *evaluationintake.Assessment
	findErr   error
	createErr error
	submitErr error
	submitted bool
}

func (s *intakeStub) FindByAnswerSheetID(context.Context, uint64) (*evaluationintake.Assessment, error) {
	*s.calls = append(*s.calls, "find")
	if s.existing != nil {
		return s.existing, nil
	}
	if s.findErr != nil {
		return nil, s.findErr
	}
	return nil, errors.New("not found")
}
func (s *intakeStub) CreateForAnswerSheet(context.Context, evaluationintake.CreateCommand) (*evaluationintake.Assessment, error) {
	*s.calls = append(*s.calls, "create")
	return s.created, s.createErr
}
func (s *intakeStub) SubmitForEvaluation(context.Context, uint64) (*evaluationintake.Assessment, error) {
	s.submitted = true
	*s.calls = append(*s.calls, "submit")
	return s.created, s.submitErr
}

type bindingStub struct {
	binding rulesetport.AssessmentBinding
	ok      bool
	err     error
}

func (s bindingStub) ResolveByQuestionnaire(context.Context, string, string) (rulesetport.Ref, bool, error) {
	return s.binding.Ref, s.ok, s.err
}

func (s bindingStub) ResolveAssessmentBinding(context.Context, string, string) (rulesetport.AssessmentBinding, bool, error) {
	return s.binding, s.ok, s.err
}

func boundScaleBinding() bindingStub {
	return bindingStub{
		binding: rulesetport.AssessmentBinding{Ref: rulesetport.Ref{
			Kind: modelcatalog.KindScale, Code: "MODEL-1", Version: "v1", Title: "model",
		}},
		ok: true,
	}
}

func TestEnsureUnboundAnswerSheetEndsWithoutCreatingAssessment(t *testing.T) {
	calls := []string{}
	intake := &intakeStub{calls: &calls, created: &evaluationintake.Assessment{ID: 91}}
	svc := NewService(scoringStub{calls: &calls}, nil, nil, nil, intake, nil)
	result, err := svc.Ensure(context.Background(), Command{OrgID: 9, AnswerSheetID: 3, QuestionnaireCode: "Q", QuestionnaireVersion: "1", TesteeID: 7, FillerID: 8})
	if err != nil {
		t.Fatal(err)
	}
	if result.AssessmentID != 0 || result.Created || result.AutoSubmitted || intake.submitted {
		t.Fatalf("result = %#v, submitted = %v", result, intake.submitted)
	}
	if !reflect.DeepEqual(calls, []string{"score", "find"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestEnsureUnboundReplayReusesLegacyAssessmentWithoutSubmit(t *testing.T) {
	calls := []string{}
	intake := &intakeStub{
		calls:    &calls,
		existing: &evaluationintake.Assessment{ID: 91, Status: "pending"},
	}
	svc := NewService(scoringStub{calls: &calls}, nil, nil, nil, intake, nil)

	result, err := svc.Ensure(context.Background(), Command{OrgID: 9, AnswerSheetID: 3, QuestionnaireCode: "Q", QuestionnaireVersion: "1", TesteeID: 7, FillerID: 8})
	if err != nil {
		t.Fatal(err)
	}
	if result.AssessmentID != 91 || result.Created || result.AutoSubmitted || intake.submitted {
		t.Fatalf("result = %#v, submitted = %v", result, intake.submitted)
	}
	if !reflect.DeepEqual(calls, []string{"score", "find"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestEnsureBoundAnswerSheetCreatesAndAutoSubmits(t *testing.T) {
	calls := []string{}
	intake := &intakeStub{
		calls:   &calls,
		created: &evaluationintake.Assessment{ID: 91, Status: "pending"},
	}
	svc := NewService(scoringStub{calls: &calls}, boundScaleBinding(), nil, nil, intake, nil)

	result, err := svc.Ensure(context.Background(), Command{OrgID: 9, AnswerSheetID: 3, QuestionnaireCode: "Q", QuestionnaireVersion: "1", TesteeID: 7, FillerID: 8})
	if err != nil {
		t.Fatal(err)
	}
	if result.AssessmentID != 91 || !result.Created || !result.AutoSubmitted || !intake.submitted {
		t.Fatalf("result = %#v, submitted = %v", result, intake.submitted)
	}
	if !reflect.DeepEqual(calls, []string{"score", "find", "create", "submit"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestEnsureTreatsScoringFailureAsHardFailure(t *testing.T) {
	calls := []string{}
	svc := NewService(scoringStub{calls: &calls, err: errors.New("score failed")}, nil, nil, nil, &intakeStub{calls: &calls}, nil)
	if _, err := svc.Ensure(context.Background(), Command{OrgID: 9, AnswerSheetID: 3, QuestionnaireCode: "Q", QuestionnaireVersion: "1", TesteeID: 7, FillerID: 8}); err == nil {
		t.Fatal("expected scoring error")
	}
	if !reflect.DeepEqual(calls, []string{"score"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestEnsureReturnsAutoSubmitFailureAfterCreation(t *testing.T) {
	calls := []string{}
	intake := &intakeStub{
		calls:     &calls,
		created:   &evaluationintake.Assessment{ID: 91, Status: "pending"},
		submitErr: errors.New("submit failed"),
	}
	svc := NewService(scoringStub{calls: &calls}, boundScaleBinding(), nil, nil, intake, nil)

	result, err := svc.Ensure(context.Background(), Command{OrgID: 9, AnswerSheetID: 3, QuestionnaireCode: "Q", QuestionnaireVersion: "1", TesteeID: 7, FillerID: 8})
	if err == nil {
		t.Fatal("expected automatic submission failure")
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if !intake.submitted || !reflect.DeepEqual(calls, []string{"score", "find", "create", "submit"}) {
		t.Fatalf("calls = %v, submitted = %v", calls, intake.submitted)
	}
}

func TestEnsureWorkerReplaySubmitsExistingBoundPendingAssessment(t *testing.T) {
	calls := []string{}
	intake := &intakeStub{
		calls:    &calls,
		existing: &evaluationintake.Assessment{ID: 91, Status: "pending"},
	}
	svc := NewService(scoringStub{calls: &calls}, boundScaleBinding(), nil, nil, intake, nil)

	result, err := svc.Ensure(context.Background(), Command{OrgID: 9, AnswerSheetID: 3, QuestionnaireCode: "Q", QuestionnaireVersion: "1", TesteeID: 7, FillerID: 8})
	if err != nil {
		t.Fatal(err)
	}
	if result.AssessmentID != 91 || result.Created || !result.AutoSubmitted || !intake.submitted {
		t.Fatalf("result = %#v, submitted = %v", result, intake.submitted)
	}
	if !reflect.DeepEqual(calls, []string{"score", "find", "submit"}) {
		t.Fatalf("calls = %v", calls)
	}
}
