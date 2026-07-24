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

type submissionDurableResultReaderStub struct {
	output *LookupAcceptedSubmissionOutput
	err    error
	calls  int
	input  *LookupAcceptedSubmissionInput
}

func (s *submissionDurableResultReaderStub) LookupAcceptedSubmission(_ context.Context, input *LookupAcceptedSubmissionInput) (*LookupAcceptedSubmissionOutput, error) {
	s.calls++
	s.input = input
	return s.output, s.err
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

type submissionCoalescerStub struct {
	contender     bool
	err           error
	ownerCalls    *int
	readbackCalls *int
}

func (s submissionCoalescerStub) Run(
	ctx context.Context,
	_ string,
	owner func(context.Context) (string, error),
	readback func(context.Context) (string, error),
) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.contender {
		if s.readbackCalls != nil {
			(*s.readbackCalls)++
		}
		return readback(ctx)
	}
	if s.ownerCalls != nil {
		(*s.ownerCalls)++
	}
	return owner(ctx)
}

type assessmentResolverStub struct {
	testeeID       uint64
	assessmentID   uint64
	readinessPhase string
	err            error
}

func (s assessmentResolverStub) ResolveAssessmentByAnswerSheetID(context.Context, uint64) (uint64, uint64, string, error) {
	return s.testeeID, s.assessmentID, s.readinessPhase, s.err
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
		writer, nil, reader, actor, submissionProfileLinkStub{enabled: true, allowed: true},
		submissionCoalescerStub{}, resolver,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second,
	)
}

func TestAcceptDurablyFailsClosedWhenProfileLinkUnavailable(t *testing.T) {
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9, IAMProfileID: "profile-7"}}
	service := NewSubmissionService(&submissionWriterStub{}, nil, nil, actor, nil, nil, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)
	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); status.Code(err) != codes.Unavailable {
		t.Fatalf("AcceptDurably() error = %v, want Unavailable", err)
	}
}

func TestAcceptDurablyFailsClosedWhenProfileLinkDisabled(t *testing.T) {
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9, IAMProfileID: "profile-7"}}
	service := NewSubmissionService(&submissionWriterStub{}, nil, nil, actor,
		submissionProfileLinkStub{enabled: false, allowed: true}, nil, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)
	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); status.Code(err) != codes.Unavailable {
		t.Fatalf("AcceptDurably() error = %v, want Unavailable", err)
	}
}

func TestAcceptDurablyChecksDisabledProfileLinkBeforeTesteeLookup(t *testing.T) {
	actorCalls := 0
	service := NewSubmissionService(&submissionWriterStub{}, nil, nil,
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
	service := NewSubmissionService(nil, nil,
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
	service := NewSubmissionService(&submissionWriterStub{}, nil, nil, actor,
		submissionProfileLinkStub{enabled: true, allowed: false}, nil, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)
	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("AcceptDurably() error = %v, want PermissionDenied", err)
	}
}

func TestAcceptDurablyMapsProfileLinkDependencyErrorToUnavailable(t *testing.T) {
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9, IAMProfileID: "profile-7"}}
	service := NewSubmissionService(&submissionWriterStub{}, nil, nil, actor,
		submissionProfileLinkStub{enabled: true, err: context.DeadlineExceeded}, nil, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)
	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); status.Code(err) != codes.Unavailable {
		t.Fatalf("AcceptDurably() error = %v, want Unavailable", err)
	}
}

func TestAcceptDurablyRejectsTesteeWithoutIAMProfile(t *testing.T) {
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9}}
	service := NewSubmissionService(&submissionWriterStub{}, nil, nil, actor,
		submissionProfileLinkStub{enabled: true, allowed: true}, nil, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)
	if _, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("AcceptDurably() error = %v, want PermissionDenied", err)
	}
}

func TestAcceptDurablyUsesDurableReadbackAfterLeaseContention(t *testing.T) {
	writer := &submissionWriterStub{}
	durableReader := &submissionDurableResultReaderStub{
		output: &LookupAcceptedSubmissionOutput{Found: true, ID: 42},
	}
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9, IAMProfileID: "profile-7"}}
	ownerCalls := 0
	readbackCalls := 0
	service := NewSubmissionService(writer, durableReader, nil, actor,
		submissionProfileLinkStub{enabled: true, allowed: true}, submissionCoalescerStub{
			contender:     true,
			ownerCalls:    &ownerCalls,
			readbackCalls: &readbackCalls,
		}, nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()}, time.Second)
	if got, err := service.AcceptDurably(t.Context(), "request-1", 11, validSubmitRequest()); err != nil || got == nil || got.ID != "42" {
		t.Fatalf("AcceptDurably() = (%#v, %v), want durable database result", got, err)
	}
	if ownerCalls != 0 || readbackCalls != 1 || durableReader.calls != 1 || writer.calls != 0 {
		t.Fatalf(
			"owner/readback/durable-reader/writer calls = %d/%d/%d/%d, want 0/1/1/0",
			ownerCalls,
			readbackCalls,
			durableReader.calls,
			writer.calls,
		)
	}
}

func TestAcceptDurablyOwnerChecksDurableResultBeforeMutableDependencies(t *testing.T) {
	durableReader := &submissionDurableResultReaderStub{
		output: &LookupAcceptedSubmissionOutput{Found: true, ID: 42},
	}
	writer := &submissionWriterStub{}
	ownerCalls := 0
	service := NewSubmissionService(
		writer,
		durableReader,
		nil,
		submissionActorStub{},
		submissionProfileLinkStub{enabled: true, err: context.DeadlineExceeded},
		submissionCoalescerStub{ownerCalls: &ownerCalls},
		nil,
		submissionQuestionnaireStub{err: status.Error(codes.Unavailable, "questionnaire down")},
		time.Second,
	)

	got, err := service.AcceptDurably(t.Context(), "request-owner-replay", 11, validSubmitRequest())
	if err != nil || got == nil || got.ID != "42" {
		t.Fatalf("AcceptDurably() = (%#v, %v), want durable replay", got, err)
	}
	if ownerCalls != 1 || durableReader.calls != 1 || writer.calls != 0 {
		t.Fatalf(
			"owner/durable-reader/writer calls = %d/%d/%d, want 1/1/0",
			ownerCalls,
			durableReader.calls,
			writer.calls,
		)
	}
}

func TestAcceptDurablyReturnsDurableReplayBeforeQuestionnaireAndProfileDependencies(t *testing.T) {
	reader := &submissionDurableResultReaderStub{
		output: &LookupAcceptedSubmissionOutput{Found: true, ID: 42},
	}
	actorCalls := 0
	writer := &submissionWriterStub{}
	service := NewSubmissionService(
		writer,
		reader,
		nil,
		submissionActorStub{calls: &actorCalls},
		submissionProfileLinkStub{enabled: true, err: context.DeadlineExceeded},
		nil,
		nil,
		submissionQuestionnaireStub{err: status.Error(codes.Unavailable, "questionnaire down")},
		time.Second,
	)

	got, err := service.AcceptDurably(t.Context(), "request-replay", 11, validSubmitRequest())
	if err != nil || got == nil || got.ID != "42" {
		t.Fatalf("AcceptDurably() = (%#v, %v), want durable replay", got, err)
	}
	if reader.calls != 1 || writer.calls != 0 || actorCalls != 0 {
		t.Fatalf("readback/writer/actor calls = %d/%d/%d, want 1/0/0", reader.calls, writer.calls, actorCalls)
	}
}

func TestAcceptDurablyReturnsReadbackConflictBeforeMutableDependencies(t *testing.T) {
	reader := &submissionDurableResultReaderStub{
		err: status.Error(codes.AlreadyExists, "idempotency conflict"),
	}
	service := NewSubmissionService(
		&submissionWriterStub{},
		reader,
		nil,
		submissionActorStub{},
		submissionProfileLinkStub{enabled: true, err: context.DeadlineExceeded},
		nil,
		nil,
		submissionQuestionnaireStub{err: status.Error(codes.Unavailable, "questionnaire down")},
		time.Second,
	)

	if _, err := service.AcceptDurably(t.Context(), "request-conflict", 11, validSubmitRequest()); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("AcceptDurably() error = %v, want AlreadyExists", err)
	}
}

func TestAcceptDurablyReadbackMissRunsFullValidationAndSave(t *testing.T) {
	reader := &submissionDurableResultReaderStub{
		output: &LookupAcceptedSubmissionOutput{},
	}
	writer := &submissionWriterStub{output: &SaveAnswerSheetOutput{ID: 42}}
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9, IAMProfileID: "profile-7"}}
	service := NewSubmissionService(
		writer,
		reader,
		nil,
		actor,
		submissionProfileLinkStub{enabled: true, allowed: true},
		nil,
		nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()},
		time.Second,
	)

	got, err := service.AcceptDurably(t.Context(), "request-miss", 11, validSubmitRequest())
	if err != nil || got == nil || got.ID != "42" {
		t.Fatalf("AcceptDurably() = (%#v, %v)", got, err)
	}
	if reader.calls != 1 || writer.calls != 1 {
		t.Fatalf("readback/writer calls = %d/%d, want 1/1", reader.calls, writer.calls)
	}
}

func TestAcceptDurablyUnimplementedReadbackFallsBackForRollingUpgrade(t *testing.T) {
	reader := &submissionDurableResultReaderStub{
		err: status.Error(codes.Unimplemented, "old apiserver"),
	}
	writer := &submissionWriterStub{output: &SaveAnswerSheetOutput{ID: 42}}
	actor := submissionActorStub{testee: &ActorTestee{OrgID: 9, IAMProfileID: "profile-7"}}
	service := NewSubmissionService(
		writer,
		reader,
		nil,
		actor,
		submissionProfileLinkStub{enabled: true, allowed: true},
		nil,
		nil,
		submissionQuestionnaireStub{questionnaire: publishedQuestionnaire()},
		time.Second,
	)

	got, err := service.AcceptDurably(t.Context(), "request-upgrade", 11, validSubmitRequest())
	if err != nil || got == nil || got.ID != "42" || writer.calls != 1 {
		t.Fatalf("AcceptDurably() = (%#v, %v), writer calls=%d", got, err, writer.calls)
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
	service := NewSubmissionService(&submissionWriterStub{}, nil, nil, actor, nil, nil, nil,
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

func TestAssessmentReadinessPendingByPhase(t *testing.T) {
	service := newAcceptService(nil, submissionReaderStub{sheet: &AnswerSheetResponse{ID: "42", TesteeID: "7"}}, assessmentResolverStub{testeeID: 7, readinessPhase: "pending"})
	got, err := service.GetAssessmentReadiness(t.Context(), 11, 42, 7)
	if err != nil {
		t.Fatalf("GetAssessmentReadiness() error = %v", err)
	}
	if got.Status != "pending" || got.NextPollAfterMs != 2000 {
		t.Fatalf("GetAssessmentReadiness() = %#v", got)
	}
}

func TestAssessmentReadinessReady(t *testing.T) {
	service := newAcceptService(nil, submissionReaderStub{sheet: &AnswerSheetResponse{ID: "42", TesteeID: "7", CreatedAt: "2026-07-18 12:34:56"}}, assessmentResolverStub{testeeID: 7, assessmentID: 99, readinessPhase: "ready"})
	got, err := service.GetAssessmentReadiness(t.Context(), 11, 42, 7)
	if err != nil {
		t.Fatalf("GetAssessmentReadiness() error = %v", err)
	}
	if got.Status != "ready" || got.AssessmentID != "99" || got.NextPollAfterMs != 0 {
		t.Fatalf("GetAssessmentReadiness() = %#v", got)
	}
}

func TestAssessmentReadinessNoAssessmentRequired(t *testing.T) {
	service := newAcceptService(nil, submissionReaderStub{sheet: &AnswerSheetResponse{ID: "42", TesteeID: "7"}}, assessmentResolverStub{testeeID: 7, readinessPhase: "no_assessment_required"})
	got, err := service.GetAssessmentReadiness(t.Context(), 11, 42, 7)
	if err != nil {
		t.Fatalf("GetAssessmentReadiness() error = %v", err)
	}
	if got.Status != "no_assessment_required" || got.AssessmentID != "" || got.NextPollAfterMs != 0 {
		t.Fatalf("GetAssessmentReadiness() = %#v", got)
	}
}

func TestAssessmentReadinessFailed(t *testing.T) {
	service := newAcceptService(nil, submissionReaderStub{sheet: &AnswerSheetResponse{ID: "42", TesteeID: "7"}}, assessmentResolverStub{testeeID: 7, assessmentID: 99, readinessPhase: "failed"})
	got, err := service.GetAssessmentReadiness(t.Context(), 11, 42, 7)
	if err != nil {
		t.Fatalf("GetAssessmentReadiness() error = %v", err)
	}
	if got.Status != "failed" || got.AssessmentID != "99" || got.NextPollAfterMs != 0 {
		t.Fatalf("GetAssessmentReadiness() = %#v", got)
	}
}

func TestAssessmentReadinessUnknownPhaseKeepsPolling(t *testing.T) {
	service := newAcceptService(nil, submissionReaderStub{sheet: &AnswerSheetResponse{ID: "42", TesteeID: "7"}}, assessmentResolverStub{testeeID: 7, readinessPhase: "weird"})
	got, err := service.GetAssessmentReadiness(t.Context(), 11, 42, 7)
	if err != nil {
		t.Fatalf("GetAssessmentReadiness() error = %v", err)
	}
	if got.Status != "pending" || got.NextPollAfterMs != 2000 {
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
