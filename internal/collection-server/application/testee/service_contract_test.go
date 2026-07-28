package testee

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/port/iamport"
)

type testeeActorContractStub struct{ created CreateTesteeInput }

func (s *testeeActorContractStub) GetTestee(context.Context, uint64) (*TesteeResponse, error) {
	return nil, nil
}
func (s *testeeActorContractStub) TesteeExists(context.Context, uint64, uint64) (bool, uint64, error) {
	return false, 0, nil
}
func (s *testeeActorContractStub) CreateTestee(_ context.Context, input CreateTesteeInput) (*TesteeResponse, error) {
	s.created = input
	return &TesteeResponse{ID: "testee-1", IAMProfileID: input.IAMProfileID}, nil
}
func (s *testeeActorContractStub) GetTesteeCareContext(context.Context, uint64) (*TesteeCareContextResponse, error) {
	return nil, nil
}
func (s *testeeActorContractStub) UpdateTestee(context.Context, uint64, *UpdateTesteeRequest) (*TesteeResponse, error) {
	return nil, nil
}
func (s *testeeActorContractStub) ListTesteesByUser(context.Context, []uint64, int32, int32) ([]*TesteeResponse, int64, error) {
	return nil, 0, nil
}

type profileCreatorContractStub struct{}

func (profileCreatorContractStub) IsEnabled() bool { return true }
func (profileCreatorContractStub) CreateProfile(context.Context, iamport.CreateProfileInput) (*iamport.CreateProfileResult, error) {
	return &iamport.CreateProfileResult{ProfileID: "profile-1", ProfileLinkID: "link-1"}, nil
}

type orgDefaultsContractStub struct{}

func (orgDefaultsContractStub) GetDefaultOrgID() uint64 { return 9 }

func TestCreateTesteeReturnsIAMProfileLinkCreatedForAuthorization(t *testing.T) {
	actor := &testeeActorContractStub{}
	service := NewService(actor, orgDefaultsContractStub{}, profileCreatorContractStub{})
	result, err := service.CreateTestee(context.Background(), 100, &CreateTesteeRequest{Name: "Child", Gender: 1, Relation: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IAMProfileID != "profile-1" || result.IAMProfileLinkID != "link-1" || actor.created.IAMUserID != "100" {
		t.Fatalf("profile/link contract was not carried through collection create: result=%+v actor=%+v", result, actor.created)
	}
}
