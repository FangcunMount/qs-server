package answersheet

import (
	"context"
	"errors"
	"testing"
	"time"

	collectionquestionnaire "github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type actorLookupClientStub struct {
	getResults map[uint64]*ActorTestee
	getErrors  map[uint64]error
	existsIDs  map[uint64]uint64
}

func (s *actorLookupClientStub) GetTestee(_ context.Context, testeeID uint64) (*ActorTestee, error) {
	if err, ok := s.getErrors[testeeID]; ok {
		return nil, err
	}
	if result, ok := s.getResults[testeeID]; ok {
		return result, nil
	}
	return nil, status.Error(codes.NotFound, "受试者不存在")
}

func (s *actorLookupClientStub) TesteeExists(_ context.Context, _ uint64, iamProfileID uint64) (bool, uint64, error) {
	if id, ok := s.existsIDs[iamProfileID]; ok {
		return true, id, nil
	}
	return false, 0, nil
}

func TestResolveCanonicalTesteeReturnsOriginalID(t *testing.T) {
	stub := &actorLookupClientStub{
		getResults: map[uint64]*ActorTestee{
			615001: {Name: "王小明"},
		},
		getErrors: map[uint64]error{},
		existsIDs: map[uint64]uint64{},
	}
	service := &SubmissionService{
		actorClient:        stub,
		profileLinkService: new(iam.ProfileLinkService),
		profileAccess:      NewProfileAccessResolver(stub, new(iam.ProfileLinkService)),
	}

	testee, resolvedID, err := service.profileAccess.resolveCanonicalTestee(context.Background(), 615001)
	if err != nil {
		t.Fatalf("resolve canonical testee: %v", err)
	}
	if resolvedID != 615001 {
		t.Fatalf("expected resolved id 615001, got %d", resolvedID)
	}
	if testee == nil || testee.Name != "王小明" {
		t.Fatalf("unexpected testee: %+v", testee)
	}
}

func TestResolveCanonicalTesteeFallsBackFromProfileID(t *testing.T) {
	const (
		profileID         = 615966157324694062
		canonicalTesteeID = 615969735435104814
	)

	stub := &actorLookupClientStub{
		getResults: map[uint64]*ActorTestee{
			canonicalTesteeID: {
				OrgID:        1,
				IAMProfileID: "615966157324694062",
				Name:         "宋博文",
			},
		},
		getErrors: map[uint64]error{
			profileID: status.Error(codes.NotFound, "受试者不存在"),
		},
		existsIDs: map[uint64]uint64{
			profileID: canonicalTesteeID,
		},
	}
	service := &SubmissionService{
		actorClient:        stub,
		profileLinkService: new(iam.ProfileLinkService),
		profileAccess:      NewProfileAccessResolver(stub, new(iam.ProfileLinkService)),
	}

	testee, resolvedID, err := service.profileAccess.resolveCanonicalTestee(context.Background(), profileID)
	if err != nil {
		t.Fatalf("resolve canonical testee with profile fallback: %v", err)
	}
	if resolvedID != canonicalTesteeID {
		t.Fatalf("expected canonical id %d, got %d", canonicalTesteeID, resolvedID)
	}
	if testee == nil || testee.Name != "宋博文" {
		t.Fatalf("unexpected canonical testee: %+v", testee)
	}
}

func TestNewSubmissionServiceAlwaysInitializesQueue(t *testing.T) {
	service := NewSubmissionService(nil, nil, nil, nil, &options.SubmitQueueOptions{
		Enabled:     false,
		QueueSize:   8,
		WorkerCount: 1,
	}, nil, nil, nil)

	if service.queue == nil {
		t.Fatal("expected submit queue to be initialized even when enabled=false")
	}
}

type submissionQuestionnaireReaderStub struct {
	questionnaire *collectionquestionnaire.QuestionnaireResponse
	err           error
}

type questionnaireCatalogReaderStub struct {
	getCalls int
}

func (s *questionnaireCatalogReaderStub) GetQuestionnaire(context.Context, string, string) (*collectionquestionnaire.QuestionnaireResponse, error) {
	s.getCalls++
	return nil, status.Error(codes.Internal, "unexpected questionnaire gRPC read")
}

func (*questionnaireCatalogReaderStub) ListQuestionnaires(context.Context, int32, int32, string, string) (*collectionquestionnaire.ListQuestionnairesResponse, error) {
	return &collectionquestionnaire.ListQuestionnairesResponse{}, nil
}

func (s submissionQuestionnaireReaderStub) Get(context.Context, string, string) (*collectionquestionnaire.QuestionnaireResponse, error) {
	return s.questionnaire, s.err
}

func TestValidateBeforeQueueRejectsInvalidAnswerWithoutQueueing(t *testing.T) {
	service := NewSubmissionService(nil, nil, nil, nil, &options.SubmitQueueOptions{QueueSize: 1, WorkerCount: 1}, nil, nil,
		submissionQuestionnaireReaderStub{questionnaire: &collectionquestionnaire.QuestionnaireResponse{
			Code: "Q", Version: "1", Status: "published",
			Questions: []collectionquestionnaire.QuestionResponse{{
				Code: "q1", Type: "Radio", Options: []collectionquestionnaire.OptionResponse{{Code: "allowed"}},
			}},
		}},
	)

	err := service.SubmitQueued(context.Background(), "req-invalid", 1, &SubmitAnswerSheetRequest{
		QuestionnaireCode: "Q", QuestionnaireVersion: "1", Answers: []Answer{{QuestionCode: "q1", QuestionType: "Radio", Value: `"blocked"`}},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SubmitQueued() error = %v, want InvalidArgument", err)
	}
	if _, ok := service.GetSubmitStatus(context.Background(), "req-invalid"); ok {
		t.Fatal("invalid answer must not create a queue status")
	}
}

func TestValidateBeforeQueueFailsClosedWhenQuestionnaireUnavailable(t *testing.T) {
	service := &SubmissionService{questionnaire: submissionQuestionnaireReaderStub{err: status.Error(codes.Unavailable, "down")}}
	err := service.validateBeforeQueue(context.Background(), &SubmitAnswerSheetRequest{QuestionnaireCode: "Q", QuestionnaireVersion: "1"})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("validateBeforeQueue() error = %v, want Unavailable", err)
	}
}

func TestValidateBeforeQueueUsesConditionalRequiredRuleFromQuestionnaireProjection(t *testing.T) {
	service := &SubmissionService{questionnaire: submissionQuestionnaireReaderStub{questionnaire: &collectionquestionnaire.QuestionnaireResponse{
		Code: "Q", Version: "1", Status: "published", Questions: []collectionquestionnaire.QuestionResponse{
			{Code: "trigger", Type: "Radio", Options: []collectionquestionnaire.OptionResponse{{Code: "yes"}, {Code: "no"}}},
			{Code: "follow", Type: "Text", ValidationRules: []collectionquestionnaire.ValidationRuleResponse{{RuleType: "required"}}, ShowController: &collectionquestionnaire.ShowControllerResponse{
				Rule: "and", Conditions: []collectionquestionnaire.ShowControllerConditionResponse{{QuestionCode: "trigger", OptionCodes: []string{"yes"}}},
			}},
		},
	}}}

	if err := service.validateBeforeQueue(context.Background(), &SubmitAnswerSheetRequest{
		QuestionnaireCode: "Q", QuestionnaireVersion: "1", Answers: []Answer{{QuestionCode: "trigger", QuestionType: "Radio", Value: `"no"`}},
	}); err != nil {
		t.Fatalf("hidden follow-up validation error = %v", err)
	}
	if err := service.validateBeforeQueue(context.Background(), &SubmitAnswerSheetRequest{
		QuestionnaireCode: "Q", QuestionnaireVersion: "1", Answers: []Answer{{QuestionCode: "trigger", QuestionType: "Radio", Value: `"yes"`}},
	}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("visible required error = %v, want InvalidArgument", err)
	}
}

func TestValidateBeforeQueueUsesExistingQuestionnaireL1Cache(t *testing.T) {
	cache := collectionquestionnaire.NewLocalCache(collectionquestionnaire.LocalCacheOptions{TTL: time.Minute, MaxEntries: 1})
	cache.Set("Q", "1", &collectionquestionnaire.QuestionnaireResponse{
		Code: "Q", Version: "1", Status: "published",
		Questions: []collectionquestionnaire.QuestionResponse{{Code: "q1", Type: "Text"}},
	})
	client := &questionnaireCatalogReaderStub{}
	service := &SubmissionService{questionnaire: collectionquestionnaire.NewQueryService(client, cache, false)}

	err := service.validateBeforeQueue(context.Background(), &SubmitAnswerSheetRequest{
		QuestionnaireCode: "Q", QuestionnaireVersion: "1",
		Answers: []Answer{{QuestionCode: "q1", QuestionType: "Text", Value: `"valid"`}},
	})
	if err != nil {
		t.Fatalf("validateBeforeQueue() error = %v", err)
	}
	if client.getCalls != 0 {
		t.Fatalf("questionnaire gRPC reads = %d, want 0 on L1 cache hit", client.getCalls)
	}
}

func TestNormalizeAnswerValueForGRPCUnwrapsRadioOptionWrapper(t *testing.T) {
	t.Parallel()

	const optionCode = "ARPkNn2y"
	got := normalizeAnswerValueForGRPC("Radio", `{"option":"`+optionCode+`"}`)
	if got != optionCode {
		t.Fatalf("normalizeAnswerValueForGRPC() = %q, want %q", got, optionCode)
	}
}

func TestNormalizeAnswerValueForGRPCLeavesNonRadioUntouched(t *testing.T) {
	t.Parallel()

	wrapped := `{"option":"ARPkNn2y"}`
	if got := normalizeAnswerValueForGRPC("Checkbox", wrapped); got != wrapped {
		t.Fatalf("normalizeAnswerValueForGRPC() = %q, want %q", got, wrapped)
	}
}

func TestConvertAnswersNormalizesRadioValuesForGRPC(t *testing.T) {
	t.Parallel()

	const optionCode = "ARPkNn2y"
	converter := AnswerConverter{}
	got := converter.Convert([]Answer{
		{
			QuestionCode: "7osLrRTA",
			QuestionType: "Radio",
			Value:        `{"option":"` + optionCode + `"}`,
		},
	})
	if len(got) != 1 {
		t.Fatalf("Convert() len = %d, want 1", len(got))
	}
	if got[0].Value != optionCode {
		t.Fatalf("Convert() value = %q, want %q", got[0].Value, optionCode)
	}
}

type idempotencyGuardStub struct {
	doneID   string
	acquired bool
	err      error
}

type leaseIdempotencyGuardStub struct {
	err error
}

func (s leaseIdempotencyGuardStub) Run(context.Context, string, func(context.Context) (string, error)) (string, bool, error) {
	return "", true, s.err
}

func (s leaseIdempotencyGuardStub) Begin(context.Context, string) (string, *locklease.Lease, bool, error) {
	return "", nil, false, errors.New("legacy Begin must not be called")
}

func (s leaseIdempotencyGuardStub) Complete(context.Context, string, *locklease.Lease, string) error {
	return errors.New("legacy Complete must not be called")
}

func (s leaseIdempotencyGuardStub) Abort(context.Context, string, *locklease.Lease) error {
	return errors.New("legacy Abort must not be called")
}

func (s *idempotencyGuardStub) Begin(context.Context, string) (string, *locklease.Lease, bool, error) {
	return s.doneID, &locklease.Lease{Key: "k", Token: "t"}, s.acquired, s.err
}

func (s *idempotencyGuardStub) Complete(context.Context, string, *locklease.Lease, string) error {
	return nil
}

func (s *idempotencyGuardStub) Abort(context.Context, string, *locklease.Lease) error {
	return nil
}

type assessmentIntakeStub struct {
	assessmentID uint64
	err          error
}

func (s *assessmentIntakeStub) EnsureAssessment(context.Context, EnsureAssessmentInput) (uint64, error) {
	return s.assessmentID, s.err
}

func (s *assessmentIntakeStub) ResolveAssessmentByAnswerSheetID(context.Context, uint64) (uint64, uint64, error) {
	return 7, s.assessmentID, s.err
}

func TestSubmitWithGuardReturnsIdempotentHit(t *testing.T) {
	t.Parallel()

	service := &SubmissionService{
		submitGuard:      &idempotencyGuardStub{doneID: "42"},
		assessmentIntake: &assessmentIntakeStub{assessmentID: 9001},
	}

	resp, err := service.Submit(context.Background(), 1, &SubmitAnswerSheetRequest{
		IdempotencyKey:    "idem-42",
		QuestionnaireCode: "QNR-001",
		TesteeID:          7,
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if resp == nil || resp.ID != "42" || resp.AssessmentID != "9001" || resp.Message != "already submitted" {
		t.Fatalf("Submit() = %#v, want idempotent hit with assessment 9001", resp)
	}
}

func TestSubmitWithLeaseGuardMapsRenewFailureToRetryableUnavailable(t *testing.T) {
	service := &SubmissionService{
		submitGuard: leaseIdempotencyGuardStub{err: locklease.ErrLeaseRenewFailed},
	}

	_, err := service.Submit(context.Background(), 1, &SubmitAnswerSheetRequest{
		IdempotencyKey: "idem-renew-failed",
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("Submit() status = %s, want Unavailable; error = %v", status.Code(err), err)
	}
	if !errors.Is(err, locklease.ErrLeaseRenewFailed) {
		t.Fatalf("Submit() error = %v, want ErrLeaseRenewFailed", err)
	}
}

func TestEffectiveIdempotencyKeyPrefersExplicitAndFallsBackToRequestID(t *testing.T) {
	original := &SubmitAnswerSheetRequest{}
	effective := withEffectiveIdempotencyKey(original, requestKey("request-42", original))
	if effective == original || effective.IdempotencyKey != "request-42" || original.IdempotencyKey != "" {
		t.Fatalf("fallback effective request = %+v original = %+v", effective, original)
	}

	explicit := &SubmitAnswerSheetRequest{IdempotencyKey: "idem-42"}
	key := requestKey("request-42", explicit)
	if key != "idem-42" || withEffectiveIdempotencyKey(explicit, key) != explicit {
		t.Fatalf("explicit idempotency key was not preferred: key=%q", key)
	}
}
