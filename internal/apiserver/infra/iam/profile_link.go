package iam

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	identityv2 "github.com/FangcunMount/iam/v2/api/grpc/iam/identity/v2"
	"github.com/FangcunMount/iam/v2/pkg/sdk/identity"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
)

// ProfileLinkService wraps IAM ProfileLink queries and commands.
type ProfileLinkService struct {
	client         *identity.ProfileLinkClient
	identityClient *identity.Client
	enabled        bool
	limiter        backpressure.Acquirer
}

// NewProfileLinkService creates a ProfileLink service wrapper.
func NewProfileLinkService(client *Client) (*ProfileLinkService, error) {
	if client == nil || !client.enabled {
		return &ProfileLinkService{enabled: false}, nil
	}

	sdkClient := client.SDK()
	if sdkClient == nil {
		return nil, fmt.Errorf("SDK client is nil")
	}

	profileLinkClient := sdkClient.ProfileLink()
	if profileLinkClient == nil {
		return nil, fmt.Errorf("profile link client is nil")
	}
	identityClient := sdkClient.Identity()
	if identityClient == nil {
		return nil, fmt.Errorf("identity client is nil")
	}

	logger.L(context.Background()).Infow("ProfileLinkService initialized",
		"component", "iam.profile_link",
		"result", "success",
	)
	return &ProfileLinkService{
		client:         profileLinkClient,
		identityClient: identityClient,
		enabled:        true,
		limiter:        client.Limiter(),
	}, nil
}

// IsEnabled 检查服务是否启用
func (s *ProfileLinkService) IsEnabled() bool {
	return s.enabled
}

// HasActiveProfileLink checks whether a user has an active link to a Profile.
func (s *ProfileLinkService) HasActiveProfileLink(ctx context.Context, userID, profileID string) (bool, error) {
	if !s.enabled {
		return false, fmt.Errorf("profile link service not enabled")
	}

	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return false, err
	}
	defer release()

	resp, err := s.client.HasProfileLink(ctx, userID, profileID)
	if err != nil {
		return false, err
	}

	return resp.GetHasProfileLink(), nil
}

// HasActiveProfileLinkWithDetails 检查用户是否拥有指定 Profile 的 active link（返回详细信息）。
func (s *ProfileLinkService) HasActiveProfileLinkWithDetails(ctx context.Context, userID, profileID string) (*identityv2.HasProfileLinkResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.HasProfileLink(ctx, userID, profileID)
}

// ValidateProfileExists 验证 IAM Profile 是否存在。
func (s *ProfileLinkService) ValidateProfileExists(ctx context.Context, profileID string) error {
	if !s.enabled {
		// IAM 未启用时，跳过验证
		logger.L(ctx).Debugw("IAM service not enabled, skip profile validation",
			"component", "iam.profile_link",
			"profile_id", profileID,
		)
		return nil
	}

	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	resp, err := s.identityClient.GetProfile(ctx, profileID)
	if err != nil {
		logger.L(ctx).Errorw("Failed to validate profile existence",
			"component", "iam.profile_link",
			"profile_id", profileID,
			"error", err.Error(),
		)
		return fmt.Errorf("failed to validate profile existence in IAM: %w", err)
	}

	if resp == nil || resp.GetProfile() == nil {
		return fmt.Errorf("profile %s does not exist in IAM system", profileID)
	}

	logger.L(ctx).Debugw("Profile validation passed",
		"component", "iam.profile_link",
		"profile_id", profileID,
	)
	return nil
}

// ListProfiles 列出用户 active ProfileLink 关联的 Profile。
func (s *ProfileLinkService) ListProfiles(ctx context.Context, userID string) (*identityv2.ListProfilesResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.ListProfiles(ctx, &identityv2.ListProfilesRequest{
		UserId: userID,
	})
}

// ListProfileLinks lists active ProfileLinks for a Profile.
func (s *ProfileLinkService) ListProfileLinks(ctx context.Context, profileID string) (*identityv2.ListProfileLinksResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.ListProfileLinks(ctx, &identityv2.ListProfileLinksRequest{
		ProfileId: profileID,
	})
}

// EstablishProfileLink creates a ProfileLink.
func (s *ProfileLinkService) EstablishProfileLink(ctx context.Context, req *identityv2.EstablishProfileLinkRequest) (*identityv2.EstablishProfileLinkResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.EstablishProfileLink(ctx, req)
}

// RevokeProfileLink revokes a ProfileLink.
func (s *ProfileLinkService) RevokeProfileLink(ctx context.Context, req *identityv2.RevokeProfileLinkRequest) (*identityv2.RevokeProfileLinkResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.RevokeProfileLink(ctx, req)
}

// BatchRevokeProfileLinks revokes ProfileLinks in batch.
func (s *ProfileLinkService) BatchRevokeProfileLinks(ctx context.Context, req *identityv2.BatchRevokeProfileLinksRequest) (*identityv2.BatchRevokeProfileLinksResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.BatchRevokeProfileLinks(ctx, req)
}

// ImportProfileLinks imports ProfileLinks in batch.
func (s *ProfileLinkService) ImportProfileLinks(ctx context.Context, req *identityv2.ImportProfileLinksRequest) (*identityv2.ImportProfileLinksResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.ImportProfileLinks(ctx, req)
}

// Raw 返回原始 SDK 客户端（用于高级用法）
func (s *ProfileLinkService) Raw() *identity.ProfileLinkClient {
	return s.client
}

func (s *ProfileLinkService) acquire(ctx context.Context) (context.Context, func(), error) {
	if s == nil || s.limiter == nil {
		return ctx, func() {}, nil
	}
	return s.limiter.Acquire(ctx)
}
