package acl

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
)

type grpcAnswerSheetWriterStub struct {
	input        *grpcbridge.SaveAnswerSheetInput
	output       *grpcbridge.SaveAnswerSheetOutput
	lookupInput  *grpcbridge.LookupAnswerSheetSubmissionInput
	lookupOutput *grpcbridge.LookupAnswerSheetSubmissionOutput
	err          error
}

func (s *grpcAnswerSheetWriterStub) SaveAnswerSheet(_ context.Context, input *grpcbridge.SaveAnswerSheetInput) (*grpcbridge.SaveAnswerSheetOutput, error) {
	s.input = input
	return s.output, s.err
}

func (s *grpcAnswerSheetWriterStub) GetAnswerSheet(context.Context, uint64) (*grpcbridge.AnswerSheetOutput, error) {
	return nil, nil
}

func (s *grpcAnswerSheetWriterStub) LookupAnswerSheetSubmission(_ context.Context, input *grpcbridge.LookupAnswerSheetSubmissionInput) (*grpcbridge.LookupAnswerSheetSubmissionOutput, error) {
	s.lookupInput = input
	return s.lookupOutput, s.err
}

func TestAnswerSheetBFFWriterMapsApplicationInputToGRPC(t *testing.T) {
	t.Parallel()

	inner := &grpcAnswerSheetWriterStub{
		output: &grpcbridge.SaveAnswerSheetOutput{ID: 8080, Message: "saved"},
	}
	writer := NewAnswerSheetBFFWriter(inner)

	got, err := writer.SaveAnswerSheet(context.Background(), &answersheet.SaveAnswerSheetInput{
		QuestionnaireCode:    "Q-1",
		QuestionnaireVersion: "v3",
		IdempotencyKey:       "idem-1",
		Title:                "测评",
		WriterID:             11,
		TesteeID:             33,
		TaskID:               "task-1",
		OrgID:                22,
		Answers: []answersheet.AnswerInput{{
			QuestionCode: "q1",
			QuestionType: "Radio",
			Score:        4,
			Value:        "A",
		}},
	})
	if err != nil {
		t.Fatalf("SaveAnswerSheet() error = %v", err)
	}
	if got == nil || got.ID != 8080 || got.Message != "saved" {
		t.Fatalf("SaveAnswerSheet() = %+v, want id=8080 message=saved", got)
	}
	input := inner.input
	if input == nil {
		t.Fatal("grpc input is nil")
	}
	if input.QuestionnaireCode != "Q-1" ||
		input.QuestionnaireVersion != "v3" ||
		input.IdempotencyKey != "idem-1" ||
		input.Title != "测评" ||
		input.WriterID != 11 ||
		input.OrgID != 22 ||
		input.TesteeID != 33 ||
		input.TaskID != "task-1" {
		t.Fatalf("unexpected grpc input: %+v", input)
	}
	if len(input.Answers) != 1 {
		t.Fatalf("answers len = %d, want 1", len(input.Answers))
	}
	if gotAnswer := input.Answers[0]; gotAnswer.QuestionCode != "q1" ||
		gotAnswer.QuestionType != "Radio" ||
		gotAnswer.Score != 4 ||
		gotAnswer.Value != "A" {
		t.Fatalf("unexpected grpc answer: %+v", gotAnswer)
	}
}

func TestAnswerSheetBFFWriterPropagatesGatewayError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("grpc failed")
	inner := &grpcAnswerSheetWriterStub{err: wantErr}
	got, err := NewAnswerSheetBFFWriter(inner).SaveAnswerSheet(context.Background(), &answersheet.SaveAnswerSheetInput{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("SaveAnswerSheet() error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatalf("SaveAnswerSheet() = %+v, want nil", got)
	}
}

func TestAnswerSheetBFFWriterAllowsNilGateway(t *testing.T) {
	t.Parallel()

	got, err := NewAnswerSheetBFFWriter(nil).SaveAnswerSheet(context.Background(), &answersheet.SaveAnswerSheetInput{})
	if err != nil {
		t.Fatalf("SaveAnswerSheet() error = %v", err)
	}
	if got != nil {
		t.Fatalf("SaveAnswerSheet() = %+v, want nil", got)
	}
}

func TestAnswerSheetDurableResultReaderMapsLookupContract(t *testing.T) {
	t.Parallel()

	inner := &grpcAnswerSheetWriterStub{
		lookupOutput: &grpcbridge.LookupAnswerSheetSubmissionOutput{Found: true, ID: 42},
	}
	reader := NewAnswerSheetDurableResultReader(inner)
	got, err := reader.LookupAcceptedSubmission(t.Context(), &answersheet.LookupAcceptedSubmissionInput{
		QuestionnaireCode:    "Q-1",
		QuestionnaireVersion: "1",
		IdempotencyKey:       "lookup-idem-1",
		WriterID:             11,
		TesteeID:             22,
		Answers: []answersheet.AnswerInput{{
			QuestionCode: "q1",
			QuestionType: "Text",
			Value:        `"ok"`,
		}},
	})
	if err != nil || got == nil || !got.Found || got.ID != 42 {
		t.Fatalf("LookupAcceptedSubmission() = (%#v, %v)", got, err)
	}
	if inner.lookupInput == nil || inner.lookupInput.WriterID != 11 || len(inner.lookupInput.Answers) != 1 {
		t.Fatalf("lookup input = %#v", inner.lookupInput)
	}
}
