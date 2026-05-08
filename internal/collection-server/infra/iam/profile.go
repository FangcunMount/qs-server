package iam

import (
	"context"
	"fmt"
	"strings"

	"github.com/FangcunMount/component-base/pkg/log"
	identityv2 "github.com/FangcunMount/iam/v2/api/grpc/iam/identity/v2"
	"github.com/FangcunMount/iam/v2/pkg/sdk/identity"
)

// ProfileService wraps IAM profile commands used by collection-server.
type ProfileService struct {
	client  *identity.ProfileClient
	enabled bool
}

type CreateProfileInput struct {
	UserID       string
	LegalName    string
	Gender       int32
	DOB          string
	IDCardNumber string
	Relation     string
}

type CreateProfileResult struct {
	ProfileID string
}

func NewProfileService(client *Client) (*ProfileService, error) {
	if client == nil || !client.enabled {
		return &ProfileService{enabled: false}, nil
	}

	sdkClient := client.SDK()
	if sdkClient == nil {
		return nil, fmt.Errorf("SDK client is nil")
	}

	profileClient := sdkClient.Profile()
	if profileClient == nil {
		return nil, fmt.Errorf("profile client is nil")
	}

	log.Info("ProfileService initialized")
	return &ProfileService{
		client:  profileClient,
		enabled: true,
	}, nil
}

func (s *ProfileService) IsEnabled() bool {
	return s != nil && s.enabled
}

func (s *ProfileService) CreateProfile(ctx context.Context, input CreateProfileInput) (*CreateProfileResult, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("profile service not enabled")
	}

	resp, err := s.client.CreateProfile(ctx, &identityv2.CreateProfileRequest{
		UserId:       input.UserID,
		LegalName:    input.LegalName,
		Gender:       toIAMGender(input.Gender),
		Dob:          input.DOB,
		IdCardNumber: input.IDCardNumber,
		Relation:     toIAMProfileRelation(input.Relation),
		Operator: &identityv2.OperatorContext{
			OperatorId: input.UserID,
			Channel:    "collection-server",
			Reason:     "create_testee",
		},
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Profile == nil || resp.Profile.Id == "" {
		return nil, fmt.Errorf("iam profile creation returned empty profile id")
	}

	return &CreateProfileResult{ProfileID: resp.Profile.Id}, nil
}

func toIAMGender(gender int32) identityv2.Gender {
	switch gender {
	case 1:
		return identityv2.Gender_GENDER_MALE
	case 2:
		return identityv2.Gender_GENDER_FEMALE
	case 3:
		return identityv2.Gender_GENDER_OTHER
	default:
		return identityv2.Gender_GENDER_UNSPECIFIED
	}
}

func toIAMProfileRelation(relation string) identityv2.ProfileLinkRelation {
	switch strings.ToLower(strings.TrimSpace(relation)) {
	case "self":
		return identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_SELF
	case "", "parent":
		return identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_PARENT
	case "grandparent":
		return identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_GRANDPARENT
	case "other":
		return identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_OTHER
	default:
		return identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_OTHER
	}
}
