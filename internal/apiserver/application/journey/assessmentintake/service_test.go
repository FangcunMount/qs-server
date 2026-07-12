package assessmentintake

import (
	"context"
	"errors"
	"reflect"
	"testing"

	evaluationintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
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
	createErr error
	submitted bool
}

func (s *intakeStub) FindByAnswerSheetID(context.Context, uint64) (*evaluationintake.Assessment, error) {
	*s.calls = append(*s.calls, "find")
	return nil, errors.New("not found")
}
func (s *intakeStub) CreateForAnswerSheet(context.Context, evaluationintake.CreateCommand) (*evaluationintake.Assessment, error) {
	*s.calls = append(*s.calls, "create")
	return s.created, s.createErr
}
func (s *intakeStub) SubmitForEvaluation(context.Context, uint64) (*evaluationintake.Assessment, error) {
	s.submitted = true
	*s.calls = append(*s.calls, "submit")
	return s.created, nil
}

func TestEnsureUnboundAnswerSheetCreatesWithoutAutoSubmit(t *testing.T) {
	calls := []string{}
	intake := &intakeStub{calls: &calls, created: &evaluationintake.Assessment{ID: 91}}
	svc := NewService(scoringStub{calls: &calls}, nil, nil, nil, intake, nil)
	result, err := svc.Ensure(context.Background(), Command{OrgID: 9, AnswerSheetID: 3, QuestionnaireCode: "Q", QuestionnaireVersion: "1", TesteeID: 7, FillerID: 8})
	if err != nil {
		t.Fatal(err)
	}
	if result.AssessmentID != 91 || !result.Created || result.AutoSubmitted || intake.submitted {
		t.Fatalf("result = %#v", result)
	}
	if !reflect.DeepEqual(calls, []string{"score", "find", "create"}) {
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
