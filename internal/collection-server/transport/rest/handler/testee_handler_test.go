package handler

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	identityv2 "github.com/FangcunMount/iam/v2/api/grpc/iam/identity/v2"
)

type userProfileReaderStub struct {
	enabled bool
	resp    *identityv2.ListProfilesResponse
	err     error
}

func (s userProfileReaderStub) IsEnabled() bool { return s.enabled }

func (s userProfileReaderStub) GetUserProfiles(context.Context, string) (*identityv2.ListProfilesResponse, error) {
	return s.resp, s.err
}

func TestLoadUserProfileIDsFailsClosedOnIAMError(t *testing.T) {
	_, err := loadUserProfileIDs(context.Background(), userProfileReaderStub{
		enabled: true,
		err:     errors.New("iam unavailable"),
	}, 42)
	if err == nil || !strings.Contains(err.Error(), "iam unavailable") {
		t.Fatalf("loadUserProfileIDs() error = %v, want IAM error", err)
	}
}

func TestLoadUserProfileIDsFailsClosedOnIncompleteResponse(t *testing.T) {
	for _, resp := range []*identityv2.ListProfilesResponse{
		nil,
		{Total: 2, Items: []*identityv2.ProfileEdge{{Profile: &identityv2.Profile{Id: "11"}}}},
		{Total: 1, Items: []*identityv2.ProfileEdge{nil}},
		{Total: 1, Items: []*identityv2.ProfileEdge{{Profile: &identityv2.Profile{Id: "invalid"}}}},
	} {
		if _, err := loadUserProfileIDs(context.Background(), userProfileReaderStub{enabled: true, resp: resp}, 42); err == nil {
			t.Fatalf("loadUserProfileIDs() response = %#v, want fail-closed error", resp)
		}
	}
}

func TestLoadUserProfileIDsReturnsCompleteScope(t *testing.T) {
	got, err := loadUserProfileIDs(context.Background(), userProfileReaderStub{
		enabled: true,
		resp: &identityv2.ListProfilesResponse{Total: 2, Items: []*identityv2.ProfileEdge{
			{Profile: &identityv2.Profile{Id: "11"}},
			{Profile: &identityv2.Profile{Id: "12"}},
		}},
	}, 42)
	if err != nil {
		t.Fatalf("loadUserProfileIDs() error = %v", err)
	}
	if want := []uint64{11, 12}; !reflect.DeepEqual(got, want) {
		t.Fatalf("loadUserProfileIDs() = %v, want %v", got, want)
	}
}
