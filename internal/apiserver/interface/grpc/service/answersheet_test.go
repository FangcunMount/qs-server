package service

import (
	"context"
	"testing"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type submissionServiceStub struct {
	submitFunc func(context.Context, appanswersheet.SubmitAnswerSheetDTO) (*appanswersheet.AnswerSheetResult, error)
}

func (s *submissionServiceStub) Submit(ctx context.Context, dto appanswersheet.SubmitAnswerSheetDTO) (*appanswersheet.AnswerSheetResult, error) {
	return s.submitFunc(ctx, dto)
}

func (s *submissionServiceStub) GetMyAnswerSheet(context.Context, uint64, uint64) (*appanswersheet.AnswerSheetResult, error) {
	return nil, nil
}

func (s *submissionServiceStub) ListMyAnswerSheets(context.Context, appanswersheet.ListMyAnswerSheetsDTO) (*appanswersheet.AnswerSheetSummaryListResult, error) {
	return nil, nil
}

type managementServiceStub struct{}

func (s *managementServiceStub) GetByID(context.Context, uint64) (*appanswersheet.AnswerSheetResult, error) {
	return nil, nil
}

func (s *managementServiceStub) List(context.Context, appanswersheet.ListAnswerSheetsDTO) (*appanswersheet.AnswerSheetSummaryListResult, error) {
	return nil, nil
}

func (s *managementServiceStub) Delete(context.Context, uint64) error {
	return nil
}

func TestAnswerSheetServiceSaveAnswerSheetDecodesStructuredValues(t *testing.T) {
	t.Parallel()

	var captured appanswersheet.SubmitAnswerSheetDTO
	svc := NewAnswerSheetService(&submissionServiceStub{
		submitFunc: func(_ context.Context, dto appanswersheet.SubmitAnswerSheetDTO) (*appanswersheet.AnswerSheetResult, error) {
			captured = dto
			return &appanswersheet.AnswerSheetResult{ID: 42}, nil
		},
	}, &managementServiceStub{})

	_, err := svc.SaveAnswerSheet(context.Background(), &pb.SaveAnswerSheetRequest{
		QuestionnaireCode:    "QNR-001",
		QuestionnaireVersion: "1.0.0",
		WriterId:             101,
		TesteeId:             202,
		Answers: []*pb.Answer{
			{QuestionCode: "q1", QuestionType: "Radio", Value: "A"},
			{QuestionCode: "q2", QuestionType: "Checkbox", Value: `["B","C"]`},
			{QuestionCode: "q3", QuestionType: "Number", Value: `12`},
		},
	})
	if err != nil {
		t.Fatalf("SaveAnswerSheet returned error: %v", err)
	}

	if len(captured.Answers) != 3 {
		t.Fatalf("expected 3 answers, got %d", len(captured.Answers))
	}

	checkboxValue, ok := captured.Answers[1].Value.([]string)
	if !ok {
		t.Fatalf("expected checkbox answer to decode into []string, got %T", captured.Answers[1].Value)
	}
	if len(checkboxValue) != 2 || checkboxValue[0] != "B" || checkboxValue[1] != "C" {
		t.Fatalf("unexpected checkbox value: %#v", checkboxValue)
	}

	numberValue, ok := captured.Answers[2].Value.(float64)
	if !ok {
		t.Fatalf("expected number answer to decode into float64, got %T", captured.Answers[2].Value)
	}
	if numberValue != 12 {
		t.Fatalf("expected numeric value 12, got %v", numberValue)
	}
}

func TestAnswerSheetServiceSaveAnswerSheetMapsInvalidDomainError(t *testing.T) {
	t.Parallel()

	svc := NewAnswerSheetService(&submissionServiceStub{
		submitFunc: func(context.Context, appanswersheet.SubmitAnswerSheetDTO) (*appanswersheet.AnswerSheetResult, error) {
			return nil, pkgerrors.WithCode(errorCode.ErrAnswerSheetInvalid, "答案验证失败")
		},
	}, &managementServiceStub{})

	_, err := svc.SaveAnswerSheet(context.Background(), &pb.SaveAnswerSheetRequest{
		QuestionnaireCode: "QNR-001",
		WriterId:          101,
		TesteeId:          202,
		Answers: []*pb.Answer{
			{QuestionCode: "q1", QuestionType: "Radio", Value: "A"},
		},
	})
	if err == nil {
		t.Fatalf("expected SaveAnswerSheet to return error")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %s", status.Code(err))
	}
}
