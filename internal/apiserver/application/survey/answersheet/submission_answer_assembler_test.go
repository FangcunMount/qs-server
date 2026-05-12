package answersheet

import (
	"context"
	"testing"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestBuildAnswerValuesAndTasksUsesSubmissionSpecQuestionType(t *testing.T) {
	t.Parallel()

	spec := mustAnswerAssemblerSubmissionSpec(t)
	results, tasks, err := buildAnswerValuesAndTasks(logger.L(context.Background()), spec, []domainQuestionnaire.RawSubmissionAnswer{
		{QuestionCode: "Q1", QuestionType: domainQuestionnaire.TypeText.Value(), Value: "hello"},
	})
	if err != nil {
		t.Fatalf("buildAnswerValuesAndTasks() error = %v", err)
	}
	if len(results) != 1 || results[0].questionType != domainQuestionnaire.TypeText {
		t.Fatalf("answer results = %+v, want text question type from spec", results)
	}
	if len(tasks) != 1 || tasks[0].ID != "Q1" {
		t.Fatalf("validation tasks = %+v, want task for Q1", tasks)
	}
}

func TestBuildAnswerValuesAndTasksRejectsDTOQuestionTypeMismatch(t *testing.T) {
	t.Parallel()

	spec := mustAnswerAssemblerSubmissionSpec(t)
	_, _, err := buildAnswerValuesAndTasks(logger.L(context.Background()), spec, []domainQuestionnaire.RawSubmissionAnswer{
		{QuestionCode: "Q1", QuestionType: domainQuestionnaire.TypeRadio.Value(), Value: "A"},
	})
	if err == nil {
		t.Fatal("buildAnswerValuesAndTasks() error = nil, want type mismatch")
	}
	if code := errors.ParseCoder(err).Code(); code != errorCode.ErrAnswerSheetInvalid {
		t.Fatalf("error code = %d, want %d", code, errorCode.ErrAnswerSheetInvalid)
	}
}

func mustAnswerAssemblerSubmissionSpec(t *testing.T) domainQuestionnaire.SubmissionSpec {
	t.Helper()
	qnr, err := domainQuestionnaire.NewQuestionnaire(
		meta.NewCode("QNR-1"),
		"Questionnaire",
		domainQuestionnaire.WithVersion(domainQuestionnaire.Version("1.0.0")),
		domainQuestionnaire.WithStatus(domainQuestionnaire.STATUS_PUBLISHED),
	)
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	question, err := domainQuestionnaire.NewQuestion(
		domainQuestionnaire.WithCode(meta.NewCode("Q1")),
		domainQuestionnaire.WithStem("Question 1"),
		domainQuestionnaire.WithQuestionType(domainQuestionnaire.TypeText),
	)
	if err != nil {
		t.Fatalf("NewQuestion() error = %v", err)
	}
	if err := qnr.AddQuestion(question); err != nil {
		t.Fatalf("AddQuestion() error = %v", err)
	}
	spec, err := qnr.BuildSubmissionSpec()
	if err != nil {
		t.Fatalf("BuildSubmissionSpec() error = %v", err)
	}
	return spec
}
