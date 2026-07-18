package service

import (
	"context"
	"testing"
	"time"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	basegrpc "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/answersheet"
	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAnswerSheetServiceMapsOwnershipFieldsToProto(t *testing.T) {
	t.Parallel()

	svc := NewAnswerSheetService(&submissionServiceStub{submitFunc: func(context.Context, appanswersheet.SubmitAnswerSheetDTO) (*appanswersheet.AnswerSheetResult, error) {
		return nil, nil
	}}, &managementServiceStub{})
	filledAt := time.Date(2026, 7, 18, 12, 34, 56, 0, time.Local)
	got := svc.toProtoAnswerSheet(&appanswersheet.AnswerSheetResult{
		ID: 42, QuestionnaireCode: "Q", QuestionnaireVer: "1.2.3", TesteeID: 77, FilledAt: filledAt,
	})

	if got.GetQuestionnaireVersion() != "1.2.3" || got.GetTesteeId() != 77 {
		t.Fatalf("toProtoAnswerSheet() = %#v", got)
	}
	if got.GetCreatedAt() != filledAt.Format("2006-01-02 15:04:05") {
		t.Fatalf("created_at = %q", got.GetCreatedAt())
	}
}

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

	ctx := context.WithValue(context.Background(), basegrpc.RequestIDContextKey, "request-propagated")
	_, err := svc.SaveAnswerSheet(ctx, &pb.SaveAnswerSheetRequest{
		QuestionnaireCode:    "QNR-001",
		QuestionnaireVersion: "1.0.0",
		IdempotencyKey:       "submit-0001",
		OrgId:                1,
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
	if captured.RequestID != "request-propagated" {
		t.Fatalf("request ID = %q, want propagated request ID", captured.RequestID)
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
		IdempotencyKey:    "submit-0002",
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

func TestAnswerSheetServiceSaveAnswerSheetRequiresSafeIdempotencyKey(t *testing.T) {
	t.Parallel()

	svc := NewAnswerSheetService(&submissionServiceStub{
		submitFunc: func(context.Context, appanswersheet.SubmitAnswerSheetDTO) (*appanswersheet.AnswerSheetResult, error) {
			t.Fatal("submission service must not be called")
			return nil, nil
		},
	}, &managementServiceStub{})

	_, err := svc.SaveAnswerSheet(context.Background(), &pb.SaveAnswerSheetRequest{
		QuestionnaireCode: "QNR-001",
		WriterId:          101,
		TesteeId:          202,
		Answers:           []*pb.Answer{{QuestionCode: "q1", QuestionType: "Radio", Value: "A"}},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %s", status.Code(err))
	}
}

func TestAnswerSheetServiceSaveAnswerSheetMapsIdempotencyConflict(t *testing.T) {
	t.Parallel()

	svc := NewAnswerSheetService(&submissionServiceStub{
		submitFunc: func(context.Context, appanswersheet.SubmitAnswerSheetDTO) (*appanswersheet.AnswerSheetResult, error) {
			return nil, pkgerrors.WithCode(errorCode.ErrConflict, "idempotency conflict")
		},
	}, &managementServiceStub{})

	_, err := svc.SaveAnswerSheet(context.Background(), &pb.SaveAnswerSheetRequest{
		QuestionnaireCode: "QNR-001",
		IdempotencyKey:    "submit-conflict",
		OrgId:             1,
		WriterId:          101,
		TesteeId:          202,
		Answers:           []*pb.Answer{{QuestionCode: "q1", QuestionType: "Radio", Value: "A"}},
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists, got %s", status.Code(err))
	}
}
