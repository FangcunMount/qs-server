package answersheet

import (
	"context"
	"errors"
	"testing"
)

type answerSheetWriterStub struct {
	input  *SaveAnswerSheetInput
	output *SaveAnswerSheetOutput
	err    error
}

func (s *answerSheetWriterStub) SaveAnswerSheet(_ context.Context, input *SaveAnswerSheetInput) (*SaveAnswerSheetOutput, error) {
	s.input = input
	return s.output, s.err
}

func TestSubmissionCommitterSendsApplicationSaveInput(t *testing.T) {
	t.Parallel()

	writer := &answerSheetWriterStub{
		output: &SaveAnswerSheetOutput{ID: 9001, Message: "ok"},
	}
	req := &SubmitAnswerSheetRequest{
		QuestionnaireCode:    "Q-1",
		QuestionnaireVersion: "v3",
		IdempotencyKey:       "idem-1",
		Title:                "测评",
		TaskID:               "task-1",
	}
	answers := []AnswerInput{{
		QuestionCode: "q1",
		QuestionType: "Radio",
		Score:        4,
		Value:        "A",
	}}

	got, err := NewSubmissionCommitter(writer).Save(context.Background(), 11, 22, 33, req, answers)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if got == nil || got.ID != 9001 || got.Message != "ok" {
		t.Fatalf("Save() = %+v, want id=9001 message=ok", got)
	}
	input := writer.input
	if input == nil {
		t.Fatal("writer input is nil")
	}
	if input.QuestionnaireCode != "Q-1" ||
		input.QuestionnaireVersion != "v3" ||
		input.IdempotencyKey != "idem-1" ||
		input.Title != "测评" ||
		input.WriterID != 11 ||
		input.OrgID != 22 ||
		input.TesteeID != 33 ||
		input.TaskID != "task-1" {
		t.Fatalf("unexpected input: %+v", input)
	}
	if len(input.Answers) != 1 || input.Answers[0] != answers[0] {
		t.Fatalf("answers = %+v, want %+v", input.Answers, answers)
	}
}

func TestSubmissionCommitterPropagatesWriterError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("save failed")
	writer := &answerSheetWriterStub{err: wantErr}

	got, err := NewSubmissionCommitter(writer).Save(
		context.Background(),
		1,
		2,
		3,
		&SubmitAnswerSheetRequest{},
		nil,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Save() error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatalf("Save() = %+v, want nil", got)
	}
}
