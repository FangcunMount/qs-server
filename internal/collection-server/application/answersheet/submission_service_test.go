package answersheet

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
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
	}, nil)

	if service.queue == nil {
		t.Fatal("expected submit queue to be initialized even when enabled=false")
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
