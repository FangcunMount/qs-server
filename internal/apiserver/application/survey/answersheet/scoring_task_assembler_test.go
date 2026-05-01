package answersheet

import (
	"testing"
	"time"

	domainActor "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestBuildAnswerScoreTasksSkipsAnswersWithoutMatchingQuestion(t *testing.T) {
	qnr := newScoringQuestionnaire(t)
	sheet := newScoringAnswerSheet(t,
		newScoringAnswer(t, "q1", "a"),
		newScoringAnswer(t, "missing", "z"),
	)

	tasks := buildAnswerScoreTasks(sheet, qnr)

	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want 1", len(tasks))
	}
	task := tasks[0]
	if task.ID != "q1" {
		t.Fatalf("task ID = %q, want q1", task.ID)
	}
	if got := task.OptionScores["a"]; got != 2 {
		t.Fatalf("option a score = %v, want 2", got)
	}
	if got := task.OptionScores["b"]; got != 5 {
		t.Fatalf("option b score = %v, want 5", got)
	}
}

func TestScoredAnswerSheetFromResultsSkipsAnswersWithoutScoreResultAndSumsScores(t *testing.T) {
	sheet := newScoringAnswerSheet(t,
		newScoringAnswer(t, "q1", "a"),
		newScoringAnswer(t, "q2", "b"),
	)

	scored := scoredAnswerSheetFromResults(sheet, []ruleengine.AnswerScoreResult{
		{ID: "q1", Score: 2, MaxScore: 5},
	})

	if scored.AnswerSheetID != 1001 {
		t.Fatalf("answer sheet ID = %d, want 1001", scored.AnswerSheetID)
	}
	if scored.TotalScore != 2 {
		t.Fatalf("total score = %v, want 2", scored.TotalScore)
	}
	if len(scored.ScoredAnswers) != 1 {
		t.Fatalf("scored answer count = %d, want 1", len(scored.ScoredAnswers))
	}
	answer := scored.ScoredAnswers[0]
	if answer.QuestionCode != "q1" || answer.Score != 2 || answer.MaxScore != 5 {
		t.Fatalf("scored answer = %+v, want q1 score 2 max 5", answer)
	}
}

func newScoringQuestionnaire(t *testing.T) *domainQuestionnaire.Questionnaire {
	t.Helper()

	qnr, err := domainQuestionnaire.NewQuestionnaire(meta.NewCode("qnr"), "Questionnaire")
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	for _, question := range []domainQuestionnaire.Question{
		newScoringQuestion(t, "q1", "a", 2, "b", 5),
		newScoringQuestion(t, "q2", "c", 3, "d", 4),
	} {
		if err := qnr.AddQuestion(question); err != nil {
			t.Fatalf("AddQuestion() error = %v", err)
		}
	}
	return qnr
}

func newScoringQuestion(t *testing.T, code, opt1 string, score1 float64, opt2 string, score2 float64) domainQuestionnaire.Question {
	t.Helper()

	question, err := domainQuestionnaire.NewQuestion(
		domainQuestionnaire.WithCode(meta.NewCode(code)),
		domainQuestionnaire.WithStem("Question "+code),
		domainQuestionnaire.WithQuestionType(domainQuestionnaire.TypeRadio),
		domainQuestionnaire.WithOption(opt1, "Option "+opt1, score1),
		domainQuestionnaire.WithOption(opt2, "Option "+opt2, score2),
	)
	if err != nil {
		t.Fatalf("NewQuestion() error = %v", err)
	}
	return question
}

func newScoringAnswerSheet(t *testing.T, answers ...domainAnswerSheet.Answer) *domainAnswerSheet.AnswerSheet {
	t.Helper()

	sheet := domainAnswerSheet.Reconstruct(
		meta.FromUint64(1001),
		domainAnswerSheet.NewQuestionnaireRef("qnr", "1.0", "Questionnaire"),
		domainActor.NewFillerRef(2001, domainActor.FillerTypeSelf),
		answers,
		time.Now(),
		0,
	)
	return sheet
}

func newScoringAnswer(t *testing.T, questionCode, optionCode string) domainAnswerSheet.Answer {
	t.Helper()

	answer, err := domainAnswerSheet.NewAnswer(
		meta.NewCode(questionCode),
		domainQuestionnaire.TypeRadio,
		domainAnswerSheet.NewOptionValue(optionCode),
		0,
	)
	if err != nil {
		t.Fatalf("NewAnswer() error = %v", err)
	}
	return answer
}
