package answersheet

import (
	"context"
	"testing"
	"time"

	collectionquestionnaire "github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type submissionActorStub struct {
	testee *ActorTestee
	calls  *int
}

func (s submissionActorStub) GetTestee(context.Context, uint64) (*ActorTestee, error) {
	if s.calls != nil {
		(*s.calls)++
	}
	if s.testee == nil {
		return nil, status.Error(codes.NotFound, "testee not found")
	}
	return s.testee, nil
}

type submissionProfileLinkStub struct {
	enabled bool
	allowed bool
	err     error
}

func (s submissionProfileLinkStub) IsEnabled() bool       { return s.enabled }
func (submissionProfileLinkStub) GetDefaultOrgID() uint64 { return 9 }
func (s submissionProfileLinkStub) HasActiveProfileLink(context.Context, string, string) (bool, error) {
	return s.allowed, s.err
}

func (submissionActorStub) TesteeExists(context.Context, uint64, uint64) (bool, uint64, error) {
	return false, 0, nil
}

type submissionWriterStub struct {
	output *SaveAnswerSheetOutput
	err    error
	calls  int
}

func (s *submissionWriterStub) SaveAnswerSheet(context.Context, *SaveAnswerSheetInput) (*SaveAnswerSheetOutput, error) {
	s.calls++
	return s.output, s.err
}

type submissionReaderStub struct {
	sheet *AnswerSheetResponse
	err   error
}

func (s submissionReaderStub) GetAnswerSheet(context.Context, uint64) (*AnswerSheetResponse, error) {
	return s.sheet, s.err
}

type submissionQuestionnaireStub struct {
	questionnaire *collectionquestionnaire.QuestionnaireResponse
	err           error
}

func (s submissionQuestionnaireStub) Get(context.Context, string, string) (*collectionquestionnaire.QuestionnaireResponse, error) {
	return s.questionnaire, s.err
}

type submissionGuardStub struct {
	acquired bool
	err      error
}

func (s submissionGuardStub) Run(ctx context.Context, _ string, body func(context.Context) (string, error)) (string, bool, error) {
	if s.err != nil || !s.acquired {
		return "", s.acquired, s.err
	}
	value, err := body(ctx)
	return value, true, err
}

type assessmentResolverStub struct {
	testeeID     uint64
	assessmentID uint64
	err          error
}

func (s assessmentResolverStub) ResolveAssessmentByAnswerSheetID(context.Context, uint64) (uint64, uint64, error) {
	return s.testeeID, s.assessmentID, s.err
}

func publishedQuestionnaire() *collectionquestionnaire.QuestionnaireResponse {
	return &collectionquestionnaire.QuestionnaireResponse{
		Code: "Q", Version: "1", Status: "published",
		Questions: []collectionquestionnaire.QuestionResponse{{Code: "q1", Type: "Text"}},
	}
}

func validSubmitRequest() *SubmitAnswerSheetRequest {
	return &SubmitAnswerSheetRequest{
		QuestionnaireCode: "Q", QuestionnaireVersion: "1", IdempotencyKey: "submit-1234", TesteeID: 7,
		Answers: []Answer{{QuestionCode: "q1", QuestionType: "Text", Value: `"answer"`}},
	}
}

func newAcceptService(writer AnswerSheetWriter, reader AnswerSheetReader, resolver AssessmentResolver) *SubmissionService {
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9, IAMProfileID: "profile-7", Name: "testee"}}
	return NewSubmissionService(
		writer, reader, actor, submissionProfileLinkStub{enabled: true, allowed: true},
		submissionGuardStub{acquired: true}, resolver,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second,
	)
}

func TestAcceptDurablyFailsClosedWhenProfileLinkUnavailable(t *testing.T) {
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9, IAMProfileID: "profile-7"}}
	service := NewSubmissionService(&submissionWriterStub{}, nil, actor, nil, nil, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)
	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); status.Code(err) != codes.Unavailable {
		t.Fatalf("AcceptDurably() error = %v, want Unavailable", err)
	}
}

func TestAcceptDurablyFailsClosedWhenProfileLinkDisabled(t *testing.T) {
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9, IAMProfileID: "profile-7"}}
	service := NewSubmissionService(&submissionWriterStub{}, nil, actor,
		submissionProfileLinkStub{enabled: false, allowed: true}, nil, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)
	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); status.Code(err) != codes.Unavailable {
		t.Fatalf("AcceptDurably() error = %v, want Unavailable", err)
	}
}

func TestAcceptDurablyChecksDisabledProfileLinkBeforeTesteeLookup(t *testing.T) {
	actorCalls := 0
	service := NewSubmissionService(&submissionWriterStub{}, nil,
		submissionActorStub{calls: &actorCalls},
		submissionProfileLinkStub{enabled: false}, nil, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)

	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); status.Code(err) != codes.Unavailable {
		t.Fatalf("AcceptDurably() error = %v, want Unavailable", err)
	}
	if actorCalls != 0 {
		t.Fatalf("actor lookup calls = %d, want 0 while ProfileLink is disabled", actorCalls)
	}
}

func TestAssessmentReadinessChecksDisabledProfileLinkBeforeTesteeLookup(t *testing.T) {
	actorCalls := 0
	service := NewSubmissionService(nil,
		submissionReaderStub{sheet: &AnswerSheetResponse{ID: "42", TesteeID: "7"}},
		submissionActorStub{calls: &actorCalls},
		submissionProfileLinkStub{enabled: false}, nil, assessmentResolverStub{}, nil, time.Second)

	if _, err := service.GetAssessmentReadiness(t.Context(), 11, 42, 7); status.Code(err) != codes.Unavailable {
		t.Fatalf("GetAssessmentReadiness() error = %v, want Unavailable", err)
	}
	if actorCalls != 0 {
		t.Fatalf("actor lookup calls = %d, want 0 while ProfileLink is disabled", actorCalls)
	}
}

func TestAcceptDurablyRejectsMissingActiveProfileLink(t *testing.T) {
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9, IAMProfileID: "profile-7"}}
	service := NewSubmissionService(&submissionWriterStub{}, nil, actor,
		submissionProfileLinkStub{enabled: true, allowed: false}, nil, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)
	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("AcceptDurably() error = %v, want PermissionDenied", err)
	}
}

func TestAcceptDurablyMapsProfileLinkDependencyErrorToUnavailable(t *testing.T) {
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9, IAMProfileID: "profile-7"}}
	service := NewSubmissionService(&submissionWriterStub{}, nil, actor,
		submissionProfileLinkStub{enabled: true, err: context.DeadlineExceeded}, nil, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)
	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); status.Code(err) != codes.Unavailable {
		t.Fatalf("AcceptDurably() error = %v, want Unavailable", err)
	}
}

func TestAcceptDurablyRejectsTesteeWithoutIAMProfile(t *testing.T) {
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9}}
	service := NewSubmissionService(&submissionWriterStub{}, nil, actor,
		submissionProfileLinkStub{enabled: true, allowed: true}, nil, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)
	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("AcceptDurably() error = %v, want PermissionDenied", err)
	}
}

func TestAcceptDurablyFallsThroughAdvisoryLeaseContention(t *testing.T) {
	writer := &submissionWriterStub{output: &SaveAnswerSheetOutput{ID: 42}}
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9, IAMProfileID: "profile-7"}}
	service := NewSubmissionService(writer, nil, actor,
		submissionProfileLinkStub{enabled: true, allowed: true}, submissionGuardStub{acquired: false}, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)
	if got, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); err != nil || got == nil || got.ID != "42" {
		t.Fatalf("AcceptDurably() = (%#v, %v), want durable database result", got, err)
	}
}

func TestAcceptDurablyReturnsAnswerSheetOnlyAfterWriterSuccess(t *testing.T) {
	writer := &submissionWriterStub{output: &SaveAnswerSheetOutput{ID: 42, Message: "saved"}}
	service := newAcceptService(writer, nil, nil)

	got, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest())
	if err != nil {
		t.Fatalf("AcceptDurably() error = %v", err)
	}
	if writer.calls != 1 || got == nil || got.ID != "42" {
		t.Fatalf("AcceptDurably() = %#v, writer calls = %d", got, writer.calls)
	}
}

func TestAcceptDurablyDoesNotRequireAssessment(t *testing.T) {
	writer := &submissionWriterStub{output: &SaveAnswerSheetOutput{ID: 42}}
	service := newAcceptService(writer, nil, assessmentResolverStub{err: status.Error(codes.Unavailable, "worker down")})

	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); err != nil {
		t.Fatalf("AcceptDurably() must not synchronously resolve assessment: %v", err)
	}
}

func TestAcceptDurablyRequiresSafeIdempotencyKey(t *testing.T) {
	service := newAcceptService(&submissionWriterStub{}, nil, nil)
	req := validSubmitRequest()
	req.IdempotencyKey = "short"
	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, req); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("AcceptDurably() error = %v, want InvalidArgument", err)
	}
}

func TestAcceptDurablyFailsClosedWhenQuestionnaireUnavailable(t *testing.T) {
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9}}
	service := NewSubmissionService(&submissionWriterStub{}, nil, actor, nil, nil, nil,
		submissionQuestionnaireStub{err: status.Error(codes.Unavailable, "down")}, time.Second)
	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); status.Code(err) != codes.Unavailable {
		t.Fatalf("AcceptDurably() error = %v, want Unavailable", err)
	}
}

func TestAssessmentReadinessPending(t *testing.T) {
	service := newAcceptService(nil, submissionReaderStub{sheet: &AnswerSheetResponse{ID: "42", TesteeID: "7"}}, assessmentResolverStub{err: status.Error(codes.NotFound, "not ready")})
	got, err := service.GetAssessmentReadiness(t.Context(), 11, 42, 7)
	if err != nil {
		t.Fatalf("GetAssessmentReadiness() error = %v", err)
	}
	if got.Status != "pending" || got.AnswerSheetID != "42" || got.NextPollAfterMs != 2000 {
		t.Fatalf("GetAssessmentReadiness() = %#v", got)
	}
}

func TestAssessmentReadinessReady(t *testing.T) {
	service := newAcceptService(nil, submissionReaderStub{sheet: &AnswerSheetResponse{ID: "42", TesteeID: "7", CreatedAt: "2026-07-18 12:34:56"}}, assessmentResolverStub{testeeID: 7, assessmentID: 99})
	got, err := service.GetAssessmentReadiness(t.Context(), 11, 42, 7)
	if err != nil {
		t.Fatalf("GetAssessmentReadiness() error = %v", err)
	}
	if got.Status != "ready" || got.AssessmentID != "99" || got.NextPollAfterMs != 0 {
		t.Fatalf("GetAssessmentReadiness() = %#v", got)
	}
}

func TestParseAnswerSheetCreatedAtSupportsCurrentAndRFC3339Formats(t *testing.T) {
	for _, value := range []string{"2026-07-18 12:34:56", "2026-07-18T12:34:56.123456789+08:00"} {
		if _, err := parseAnswerSheetCreatedAt(value); err != nil {
			t.Fatalf("parseAnswerSheetCreatedAt(%q) error = %v", value, err)
		}
	}
}

func TestAssessmentReadinessRejectsMismatchedTestee(t *testing.T) {
	service := newAcceptService(nil, submissionReaderStub{sheet: &AnswerSheetResponse{ID: "42", TesteeID: "7"}}, assessmentResolverStub{})
	if _, err := service.GetAssessmentReadiness(t.Context(), 11, 42, 8); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("GetAssessmentReadiness() error = %v, want PermissionDenied", err)
	}
}
