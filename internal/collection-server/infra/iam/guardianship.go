package iam

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/log"
	identityv2 "github.com/FangcunMount/iam/v2/api/grpc/iam/identity/v2"
	"github.com/FangcunMount/iam/v2/pkg/sdk/identity"
)

// GuardianshipService 监护关系服务封装
// 提供监护关系查询和管理功能
type GuardianshipService struct {
	client  *identity.ProfileLinkClient
	enabled bool
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

	log.Info("GuardianshipService initialized")
	return &GuardianshipService{
		client:  profileLinkClient,
		enabled: true,
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
	return s.client.HasProfileLink(ctx, userID, childID)
}

// ListChildren 列出用户的所有被监护儿童
func (s *GuardianshipService) ListChildren(ctx context.Context, userID string) (*identityv2.ListProfilesResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	return s.client.ListProfiles(ctx, &identityv2.ListProfilesRequest{
		UserId: userID,
	})
}

// GetUserChildren 获取用户的所有儿童（便捷方法，SDK v0.0.6 新增）
// 这是 ListChildren 的快捷方式，直接传入 userID 字符串
func (s *GuardianshipService) GetUserChildren(ctx context.Context, userID string) (*identityv2.ListProfilesResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	return s.client.GetUserProfiles(ctx, userID)
}

// ListGuardians 列出儿童的所有监护人
func (s *GuardianshipService) ListGuardians(ctx context.Context, childID string) (*identityv2.ListProfileLinksResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	return s.client.ListProfileLinks(ctx, &identityv2.ListProfileLinksRequest{
		ProfileId: childID,
	})
}

// EstablishProfileLink 添加监护关系。
func (s *GuardianshipService) EstablishProfileLink(ctx context.Context, req *identityv2.EstablishProfileLinkRequest) (*identityv2.EstablishProfileLinkResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	return s.client.EstablishProfileLink(ctx, req)
}

// RevokeProfileLink 撤销监护关系。
func (s *GuardianshipService) RevokeProfileLink(ctx context.Context, req *identityv2.RevokeProfileLinkRequest) (*identityv2.RevokeProfileLinkResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	return s.client.RevokeProfileLink(ctx, req)
}

// BatchRevokeProfileLinks 批量撤销监护关系。
func (s *GuardianshipService) BatchRevokeProfileLinks(ctx context.Context, req *identityv2.BatchRevokeProfileLinksRequest) (*identityv2.BatchRevokeProfileLinksResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	return s.client.BatchRevokeProfileLinks(ctx, req)
}

// ImportProfileLinks 批量导入监护关系。
func (s *GuardianshipService) ImportProfileLinks(ctx context.Context, req *identityv2.ImportProfileLinksRequest) (*identityv2.ImportProfileLinksResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	return s.client.ImportProfileLinks(ctx, req)
}

// GetDefaultOrgID 获取默认机构ID
// 在单租户场景下，返回固定的机构ID
// TODO: 未来如果需要多租户支持，可以通过 IAM SDK 获取用户所属机构
func (s *GuardianshipService) GetDefaultOrgID() uint64 {
	return 1
}

// Raw 返回原始 SDK 客户端（用于高级用法）
func (s *GuardianshipService) Raw() *identity.ProfileLinkClient {
	return s.client
}
