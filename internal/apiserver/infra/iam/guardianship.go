package iam

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	identityv2 "github.com/FangcunMount/iam/v2/api/grpc/iam/identity/v2"
	"github.com/FangcunMount/iam/v2/pkg/sdk/identity"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
)

// GuardianshipService 监护关系服务封装
// 提供监护关系查询和管理功能
type GuardianshipService struct {
	client         *identity.ProfileLinkClient
	identityClient *identity.Client
	enabled        bool
	limiter        backpressure.Acquirer
}

// NewGuardianshipService 创建监护关系服务
func NewGuardianshipService(client *Client) (*GuardianshipService, error) {
	if client == nil || !client.enabled {
		return &GuardianshipService{enabled: false}, nil
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

	logger.L(context.Background()).Infow("GuardianshipService initialized",
		"component", "iam.guardianship",
		"result", "success",
	)
	return &GuardianshipService{
		client:         profileLinkClient,
		identityClient: identityClient,
		enabled:        true,
		limiter:        client.Limiter(),
	}, nil
}

// IsEnabled 检查服务是否启用
func (s *GuardianshipService) IsEnabled() bool {
	return s.enabled
}

// IsGuardian 检查是否是监护人
// 用于权限验证场景
func (s *GuardianshipService) IsGuardian(ctx context.Context, userID, childID string) (bool, error) {
	if !s.enabled {
		return false, fmt.Errorf("guardianship service not enabled")
	}

	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return false, err
	}
	defer release()

	resp, err := s.client.HasProfileLink(ctx, userID, childID)
	if err != nil {
		return false, err
	}

	return resp.GetHasProfileLink(), nil
}

// IsGuardianWithDetails 检查是否是监护人（返回详细信息）
func (s *GuardianshipService) IsGuardianWithDetails(ctx context.Context, userID, childID string) (*identityv2.HasProfileLinkResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.HasProfileLink(ctx, userID, childID)
}

// ValidateChildExists 验证儿童是否存在
// 通过查询该 childID 的监护人列表来判断儿童是否存在
// 如果 childID 不存在，IAM 系统会返回错误或空列表
func (s *GuardianshipService) ValidateChildExists(ctx context.Context, childID string) error {
	if !s.enabled {
		// IAM 未启用时，跳过验证
		logger.L(ctx).Debugw("IAM service not enabled, skip child validation",
			"component", "iam.guardianship",
			"child_id", childID,
		)
		return nil
	}

	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	resp, err := s.identityClient.GetProfile(ctx, childID)
	if err != nil {
		logger.L(ctx).Errorw("Failed to validate child existence",
			"component", "iam.guardianship",
			"child_id", childID,
			"error", err.Error(),
		)
		return fmt.Errorf("failed to validate child existence in IAM: %w", err)
	}

	if resp == nil || resp.GetProfile() == nil {
		return fmt.Errorf("profile %s does not exist in IAM system", childID)
	}

	logger.L(ctx).Debugw("Child validation passed",
		"component", "iam.guardianship",
		"child_id", childID,
	)
	return nil
}

// ListChildren 列出用户的所有被监护儿童
func (s *GuardianshipService) ListChildren(ctx context.Context, userID string) (*identityv2.ListProfilesResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
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

// ListGuardians 列出儿童的所有监护人
func (s *GuardianshipService) ListGuardians(ctx context.Context, childID string) (*identityv2.ListProfileLinksResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.ListProfileLinks(ctx, &identityv2.ListProfileLinksRequest{
		ProfileId: childID,
	})
}

// EstablishProfileLink 添加监护关系。
func (s *GuardianshipService) EstablishProfileLink(ctx context.Context, req *identityv2.EstablishProfileLinkRequest) (*identityv2.EstablishProfileLinkResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.EstablishProfileLink(ctx, req)
}

// RevokeProfileLink 撤销监护关系。
func (s *GuardianshipService) RevokeProfileLink(ctx context.Context, req *identityv2.RevokeProfileLinkRequest) (*identityv2.RevokeProfileLinkResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.RevokeProfileLink(ctx, req)
}

// BatchRevokeProfileLinks 批量撤销监护关系。
func (s *GuardianshipService) BatchRevokeProfileLinks(ctx context.Context, req *identityv2.BatchRevokeProfileLinksRequest) (*identityv2.BatchRevokeProfileLinksResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.BatchRevokeProfileLinks(ctx, req)
}

// ImportProfileLinks 批量导入监护关系。
func (s *GuardianshipService) ImportProfileLinks(ctx context.Context, req *identityv2.ImportProfileLinksRequest) (*identityv2.ImportProfileLinksResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.ImportProfileLinks(ctx, req)
}

// Raw 返回原始 SDK 客户端（用于高级用法）
func (s *GuardianshipService) Raw() *identity.ProfileLinkClient {
	return s.client
}

func (s *GuardianshipService) acquire(ctx context.Context) (context.Context, func(), error) {
	if s == nil || s.limiter == nil {
		return ctx, func() {}, nil
	}
	return s.limiter.Acquire(ctx)
}
