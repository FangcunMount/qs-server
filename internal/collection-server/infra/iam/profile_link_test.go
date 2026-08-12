package iam

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	identityv2 "github.com/FangcunMount/iam/v2/api/grpc/iam/identity/v2"
)

func TestGetUserProfilesLoadsEveryIAMPage(t *testing.T) {
	var offsets []uint32
	service := &ProfileLinkService{
		enabled: true,
		listProfiles: func(_ context.Context, req *identityv2.ListProfilesRequest) (*identityv2.ListProfilesResponse, error) {
			offsets = append(offsets, req.GetPage().GetOffset())
			switch req.GetPage().GetOffset() {
			case 0:
				return &identityv2.ListProfilesResponse{Total: 3, Items: []*identityv2.ProfileEdge{
					{Profile: &identityv2.Profile{Id: "11"}},
					{Profile: &identityv2.Profile{Id: "12"}},
				}}, nil
			case 2:
				return &identityv2.ListProfilesResponse{Total: 3, Items: []*identityv2.ProfileEdge{
					{Profile: &identityv2.Profile{Id: "13"}},
				}}, nil
			default:
				return nil, fmt.Errorf("unexpected offset %d", req.GetPage().GetOffset())
			}
		},
	}

	resp, err := service.GetUserProfiles(context.Background(), "42")
	if err != nil {
		t.Fatalf("GetUserProfiles() error = %v", err)
	}
	if resp.Total != 3 || len(resp.Items) != 3 {
		t.Fatalf("GetUserProfiles() total=%d items=%d, want 3", resp.Total, len(resp.Items))
	}
	if want := []uint32{0, 2}; !reflect.DeepEqual(offsets, want) {
		t.Fatalf("pagination offsets = %v, want %v", offsets, want)
	}
}

func TestGetUserProfilesFailsClosedOnIncompletePagination(t *testing.T) {
	service := &ProfileLinkService{
		enabled: true,
		listProfiles: func(context.Context, *identityv2.ListProfilesRequest) (*identityv2.ListProfilesResponse, error) {
			return &identityv2.ListProfilesResponse{Total: 2}, nil
		},
	}
	_, err := service.GetUserProfiles(context.Background(), "42")
	if err == nil || !strings.Contains(err.Error(), "stopped before total") {
		t.Fatalf("GetUserProfiles() error = %v, want incomplete pagination error", err)
	}
}
