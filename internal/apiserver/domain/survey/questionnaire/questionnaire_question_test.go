package questionnaire

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestQuestionnaireAddUpdateRemoveQuestion(t *testing.T) {
	t.Parallel()

	q := newTestQuestionnaire(t)
	question := newTestQuestion(t, "Q1", "Question 1")

	if err := q.AddQuestion(question); err != nil {
		t.Fatalf("AddQuestion() error = %v", err)
	}
	if err := q.AddQuestion(question); err == nil {
		t.Fatal("expected duplicate question error")
	}

	updated := newTestQuestion(t, "Q1", "Updated Question 1")
	if err := q.UpdateQuestion(updated); err != nil {
		t.Fatalf("UpdateQuestion() error = %v", err)
	}
	got, ok := q.GetQuestionByCode(meta.NewCode("Q1"))
	if !ok {
		t.Fatal("expected updated question")
	}
	if got.GetStem() != "Updated Question 1" {
		t.Fatalf("question stem = %q, want Updated Question 1", got.GetStem())
	}

	if err := q.RemoveQuestion(meta.NewCode("Q1")); err != nil {
		t.Fatalf("RemoveQuestion() error = %v", err)
	}
	if q.QuestionCount() != 0 {
		t.Fatalf("question count = %d, want 0", q.QuestionCount())
	}
}

func TestQuestionnaireReplaceQuestionsRejectsDuplicateCode(t *testing.T) {
	t.Parallel()

	q := newTestQuestionnaire(t)
	if err := q.ReplaceQuestions([]Question{
		newTestQuestion(t, "Q1", "Question 1"),
		newTestQuestion(t, "Q1", "Duplicate Question 1"),
	}); err == nil {
		t.Fatal("expected duplicate question error")
	}
}

func TestQuestionnaireReorderQuestions(t *testing.T) {
	t.Parallel()

	q := newTestQuestionnaire(t)
	if err := q.ReplaceQuestions([]Question{
		newTestQuestion(t, "Q1", "Question 1"),
		newTestQuestion(t, "Q2", "Question 2"),
	}); err != nil {
		t.Fatalf("ReplaceQuestions() error = %v", err)
	}

	if err := q.ReorderQuestions([]meta.Code{meta.NewCode("Q2"), meta.NewCode("Q1")}); err != nil {
		t.Fatalf("ReorderQuestions() error = %v", err)
	}
	questions := q.GetQuestions()
	if questions[0].GetCode().Value() != "Q2" || questions[1].GetCode().Value() != "Q1" {
		t.Fatalf("question order = [%s,%s], want [Q2,Q1]", questions[0].GetCode().Value(), questions[1].GetCode().Value())
	}

	if err := q.ReorderQuestions([]meta.Code{meta.NewCode("Q1")}); err == nil {
		t.Fatal("expected length mismatch error")
	}
}

func newTestQuestionnaire(t *testing.T) *Questionnaire {
	t.Helper()

	q, err := NewQuestionnaire(meta.NewCode("QN_A"), "Questionnaire A")
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	return q
}

func newTestQuestion(t *testing.T, code, stem string) Question {
	t.Helper()

	q, err := NewQuestion(
		WithCode(meta.NewCode(code)),
		WithStem(stem),
		WithQuestionType(TypeSection),
	)
	if err != nil {
		t.Fatalf("NewQuestion() error = %v", err)
	}
	return q
}
