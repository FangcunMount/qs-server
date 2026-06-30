package answersheet

import (
	"context"
	"testing"

	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	ruleengineport "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestBuildAnswerScoreTasksUnwrapsMiniProgramOptionWrapper(t *testing.T) {
	t.Parallel()

	const optionCode = "ARPkNn2y"
	qnr := newWrappedOptionQuestionnaire(t, optionCode, 2)
	sheet := newScoringAnswerSheet(t, newWrappedOptionAnswer(t, "7osLrRTA", optionCode))

	tasks := buildAnswerScoreTasks(sheet, qnr)
	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want 1", len(tasks))
	}

	results, err := optionCodeScorerStub{}.ScoreAnswers(context.Background(), tasks)
	if err != nil {
		t.Fatalf("ScoreAnswers() error = %v", err)
	}
	if len(results) != 1 || results[0].Score != 2 {
		t.Fatalf("results = %+v, want score 2", results)
	}
}

func TestBuildAnswerScoreTasksScoresStoredWrapperLiteral(t *testing.T) {
	t.Parallel()

	const optionCode = "ARPkNn2y"
	qnr := newWrappedOptionQuestionnaire(t, optionCode, 3)
	answer, err := domainAnswerSheet.NewAnswer(
		meta.NewCode("7osLrRTA"),
		domainQuestionnaire.TypeRadio,
		domainAnswerSheet.NewOptionValue(`{"option":"`+optionCode+`"}`),
		0,
	)
	if err != nil {
		t.Fatalf("NewAnswer() error = %v", err)
	}
	sheet := newScoringAnswerSheet(t, answer)

	results, err := optionCodeScorerStub{}.ScoreAnswers(context.Background(), buildAnswerScoreTasks(sheet, qnr))
	if err != nil {
		t.Fatalf("ScoreAnswers() error = %v", err)
	}
	if len(results) != 1 || results[0].Score != 3 {
		t.Fatalf("results = %+v, want score 3", results)
	}
}

type optionCodeScorerStub struct{}

func (optionCodeScorerStub) ScoreAnswers(_ context.Context, tasks []ruleengineport.AnswerScoreTask) ([]ruleengineport.AnswerScoreResult, error) {
	results := make([]ruleengineport.AnswerScoreResult, 0, len(tasks))
	for _, task := range tasks {
		score := 0.0
		if selected, ok := task.Value.AsSingleSelection(); ok {
			score = task.OptionScores[selected]
		}
		results = append(results, ruleengineport.AnswerScoreResult{
			ID:    task.ID,
			Score: score,
		})
	}
	return results, nil
}

func newWrappedOptionQuestionnaire(t *testing.T, optionCode string, score float64) *domainQuestionnaire.Questionnaire {
	t.Helper()

	qnr, err := domainQuestionnaire.NewQuestionnaire(meta.NewCode("3adyDE"), "SNAP")
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	question, err := domainQuestionnaire.NewQuestion(
		domainQuestionnaire.WithCode(meta.NewCode("7osLrRTA")),
		domainQuestionnaire.WithStem("Question"),
		domainQuestionnaire.WithQuestionType(domainQuestionnaire.TypeRadio),
		domainQuestionnaire.WithOption(optionCode, "有一点", score),
	)
	if err != nil {
		t.Fatalf("NewQuestion() error = %v", err)
	}
	if err := qnr.AddQuestion(question); err != nil {
		t.Fatalf("AddQuestion() error = %v", err)
	}
	return qnr
}

func newWrappedOptionAnswer(t *testing.T, questionCode, optionCode string) domainAnswerSheet.Answer {
	t.Helper()

	value, err := domainAnswerSheet.CreateAnswerValueFromRaw(
		domainQuestionnaire.TypeRadio,
		`{"option":"`+optionCode+`"}`,
	)
	if err != nil {
		t.Fatalf("CreateAnswerValueFromRaw() error = %v", err)
	}
	answer, err := domainAnswerSheet.NewAnswer(
		meta.NewCode(questionCode),
		domainQuestionnaire.TypeRadio,
		value,
		0,
	)
	if err != nil {
		t.Fatalf("NewAnswer() error = %v", err)
	}
	return answer
}
