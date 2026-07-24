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
	lookupFunc func(context.Context, appanswersheet.LookupSubmissionDTO) (*appanswersheet.AnswerSheetResult, bool, error)
}

func (s *submissionServiceStub) Submit(ctx context.Context, dto appanswersheet.SubmitAnswerSheetDTO) (*appanswersheet.AnswerSheetResult, error) {
	return s.submitFunc(ctx, dto)
}

func (s *submissionServiceStub) LookupAcceptedSubmission(ctx context.Context, dto appanswersheet.LookupSubmissionDTO) (*appanswersheet.AnswerSheetResult, bool, error) {
	if s.lookupFunc == nil {
		return nil, false, nil
	}
	return s.lookupFunc(ctx, dto)
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

func TestAnswerSheetServiceLookupAnswerSheetSubmissionReturnsDurableHit(t *testing.T) {
	t.Parallel()

	var captured appanswersheet.LookupSubmissionDTO
	svc := NewAnswerSheetService(&submissionServiceStub{
		submitFunc: func(context.Context, appanswersheet.SubmitAnswerSheetDTO) (*appanswersheet.AnswerSheetResult, error) {
			t.Fatal("lookup must not call Submit")
			return nil, nil
		},
		lookupFunc: func(_ context.Context, dto appanswersheet.LookupSubmissionDTO) (*appanswersheet.AnswerSheetResult, bool, error) {
			captured = dto
			return &appanswersheet.AnswerSheetResult{ID: 42}, true, nil
		},
	}, &managementServiceStub{})

	response, err := svc.LookupAnswerSheetSubmission(t.Context(), &pb.LookupAnswerSheetSubmissionRequest{
		WriterId:             101,
		IdempotencyKey:       "submit-lookup-1",
		QuestionnaireCode:    "QNR-001",
		QuestionnaireVersion: "1.0.0",
		TesteeId:             202,
		Answers:              []*pb.SubmissionIntentAnswer{{QuestionCode: "q1", QuestionType: "Checkbox", Value: `["A","B"]`}},
	})
	if err != nil || response == nil || !response.Found || response.Id != 42 {
		t.Fatalf("LookupAnswerSheetSubmission() = response=%#v err=%v", response, err)
	}
	if len(captured.Answers) != 1 {
		t.Fatalf("captured answers = %#v", captured.Answers)
	}
	if _, ok := captured.Answers[0].Value.([]string); !ok {
		t.Fatalf("captured answer value type = %T, want []string", captured.Answers[0].Value)
	}
}

func TestAnswerSheetServiceLookupAnswerSheetSubmissionReturnsExplicitMiss(t *testing.T) {
	t.Parallel()

	svc := NewAnswerSheetService(&submissionServiceStub{
		submitFunc: func(context.Context, appanswersheet.SubmitAnswerSheetDTO) (*appanswersheet.AnswerSheetResult, error) {
			return nil, nil
		},
		lookupFunc: func(context.Context, appanswersheet.LookupSubmissionDTO) (*appanswersheet.AnswerSheetResult, bool, error) {
			return nil, false, nil
		},
	}, &managementServiceStub{})

	response, err := svc.LookupAnswerSheetSubmission(t.Context(), validLookupSubmissionRequest())
	if err != nil || response == nil || response.Found || response.Id != 0 {
		t.Fatalf("LookupAnswerSheetSubmission() = response=%#v err=%v", response, err)
	}
}

func TestAnswerSheetServiceLookupAnswerSheetSubmissionMapsConflict(t *testing.T) {
	t.Parallel()

	svc := NewAnswerSheetService(&submissionServiceStub{
		submitFunc: func(context.Context, appanswersheet.SubmitAnswerSheetDTO) (*appanswersheet.AnswerSheetResult, error) {
			return nil, nil
		},
		lookupFunc: func(context.Context, appanswersheet.LookupSubmissionDTO) (*appanswersheet.AnswerSheetResult, bool, error) {
			return nil, false, pkgerrors.WithCode(errorCode.ErrConflict, "idempotency conflict")
		},
	}, &managementServiceStub{})

	_, err := svc.LookupAnswerSheetSubmission(t.Context(), validLookupSubmissionRequest())
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("LookupAnswerSheetSubmission() status = %s, want AlreadyExists", status.Code(err))
	}
}

func TestAnswerSheetServiceLookupAnswerSheetSubmissionMapsReadErrorToUnavailable(t *testing.T) {
	t.Parallel()

	svc := NewAnswerSheetService(&submissionServiceStub{
		lookupFunc: func(context.Context, appanswersheet.LookupSubmissionDTO) (*appanswersheet.AnswerSheetResult, bool, error) {
			return nil, false, pkgerrors.WithCode(errorCode.ErrDatabase, "mongo unavailable")
		},
	}, &managementServiceStub{})

	_, err := svc.LookupAnswerSheetSubmission(t.Context(), validLookupSubmissionRequest())
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("LookupAnswerSheetSubmission() status = %s, want Unavailable", status.Code(err))
	}
}

func TestAnswerSheetServiceLookupAnswerSheetSubmissionPreservesCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := NewAnswerSheetService(&submissionServiceStub{
		lookupFunc: func(context.Context, appanswersheet.LookupSubmissionDTO) (*appanswersheet.AnswerSheetResult, bool, error) {
			return nil, false, context.Canceled
		},
	}, &managementServiceStub{})

	_, err := svc.LookupAnswerSheetSubmission(ctx, validLookupSubmissionRequest())
	if status.Code(err) != codes.Canceled {
		t.Fatalf("LookupAnswerSheetSubmission() status = %s, want Canceled", status.Code(err))
	}
}

func TestAnswerSheetServiceLookupAnswerSheetSubmissionRejectsFoundWithoutID(t *testing.T) {
	t.Parallel()

	svc := NewAnswerSheetService(&submissionServiceStub{
		lookupFunc: func(context.Context, appanswersheet.LookupSubmissionDTO) (*appanswersheet.AnswerSheetResult, bool, error) {
			return &appanswersheet.AnswerSheetResult{}, true, nil
		},
	}, &managementServiceStub{})

	_, err := svc.LookupAnswerSheetSubmission(t.Context(), validLookupSubmissionRequest())
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("LookupAnswerSheetSubmission() status = %s, want Unavailable", status.Code(err))
	}
}

func validLookupSubmissionRequest() *pb.LookupAnswerSheetSubmissionRequest {
	return &pb.LookupAnswerSheetSubmissionRequest{
		WriterId:             101,
		IdempotencyKey:       "submit-lookup-1",
		QuestionnaireCode:    "QNR-001",
		QuestionnaireVersion: "1.0.0",
		TesteeId:             202,
		Answers:              []*pb.SubmissionIntentAnswer{{QuestionCode: "q1", QuestionType: "Text", Value: `"ok"`}},
	}
}
