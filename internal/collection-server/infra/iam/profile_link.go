package iam

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/log"
	identityv2 "github.com/FangcunMount/iam/v3/api/grpc/iam/identity/v2"
	"github.com/FangcunMount/iam/v3/pkg/sdk/identity"
)

// ProfileLinkService wraps IAM ProfileLink queries and commands.
type ProfileLinkService struct {
	client       *identity.ProfileLinkClient
	listProfiles func(context.Context, *identityv2.ListProfilesRequest) (*identityv2.ListProfilesResponse, error)
	enabled      bool
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

	log.Info("ProfileLinkService initialized")
	return &ProfileLinkService{
		client:       profileLinkClient,
		listProfiles: profileLinkClient.ListProfiles,
		enabled:      true,
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
	return s.client.HasProfileLink(ctx, userID, profileID)
}

// ListProfiles 列出用户 active ProfileLink 关联的 Profile。
func (s *ProfileLinkService) ListProfiles(ctx context.Context, userID string) (*identityv2.ListProfilesResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	return s.client.ListProfiles(ctx, &identityv2.ListProfilesRequest{
		UserId: userID,
	})
}

// GetUserProfiles 获取用户 active ProfileLink 关联的 Profile。
func (s *ProfileLinkService) GetUserProfiles(ctx context.Context, userID string) (*identityv2.ListProfilesResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	listProfiles := s.listProfiles
	if listProfiles == nil {
		if s.client == nil {
			return nil, fmt.Errorf("profile link client is nil")
		}
		listProfiles = s.client.ListProfiles
	}
	const pageLimit uint32 = 50
	allItems := make([]*identityv2.ProfileEdge, 0, pageLimit)
	var total int32 = -1
	for offset := uint32(0); ; {
		resp, err := listProfiles(ctx, &identityv2.ListProfilesRequest{
			UserId: userID,
			Page:   &identityv2.OffsetPagination{Offset: offset, Limit: pageLimit},
		})
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, fmt.Errorf("list user profiles returned nil response at offset %d", offset)
		}
		if total < 0 {
			total = resp.Total
		} else if resp.Total != total {
			return nil, fmt.Errorf("list user profiles total changed during pagination: before=%d now=%d", total, resp.Total)
		}
		allItems = append(allItems, resp.Items...)
		if int32(len(allItems)) >= total {
			break
		}
		if len(resp.Items) == 0 {
			return nil, fmt.Errorf("list user profiles stopped before total: total=%d collected=%d", total, len(allItems))
		}
		offset += uint32(len(resp.Items))
	}
	if int32(len(allItems)) != total {
		return nil, fmt.Errorf("list user profiles count mismatch: total=%d collected=%d", total, len(allItems))
	}
	return &identityv2.ListProfilesResponse{
		Total: total,
		Page:  &identityv2.OffsetPagination{Offset: 0, Limit: uint32(len(allItems))},
		Items: allItems,
	}, nil
}

// ListProfileLinks lists active ProfileLinks for a Profile.
func (s *ProfileLinkService) ListProfileLinks(ctx context.Context, profileID string) (*identityv2.ListProfileLinksResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	return s.client.ListProfileLinks(ctx, &identityv2.ListProfileLinksRequest{
		ProfileId: profileID,
	})
}

// EstablishProfileLink creates a ProfileLink.
func (s *ProfileLinkService) EstablishProfileLink(ctx context.Context, req *identityv2.EstablishProfileLinkRequest) (*identityv2.EstablishProfileLinkResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	return s.client.EstablishProfileLink(ctx, req)
}

// RevokeProfileLink revokes a ProfileLink.
func (s *ProfileLinkService) RevokeProfileLink(ctx context.Context, req *identityv2.RevokeProfileLinkRequest) (*identityv2.RevokeProfileLinkResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	return s.client.RevokeProfileLink(ctx, req)
}

// BatchRevokeProfileLinks revokes ProfileLinks in batch.
func (s *ProfileLinkService) BatchRevokeProfileLinks(ctx context.Context, req *identityv2.BatchRevokeProfileLinksRequest) (*identityv2.BatchRevokeProfileLinksResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	return s.client.BatchRevokeProfileLinks(ctx, req)
}

// ImportProfileLinks imports ProfileLinks in batch.
func (s *ProfileLinkService) ImportProfileLinks(ctx context.Context, req *identityv2.ImportProfileLinksRequest) (*identityv2.ImportProfileLinksResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	return s.client.ImportProfileLinks(ctx, req)
}

// GetDefaultOrgID 获取默认机构ID
// 当前部署采用 org=1 的单租户契约；多租户切换必须先引入显式机构解析端口，不能在此处隐式猜测。
func (s *ProfileLinkService) GetDefaultOrgID() uint64 {
	return 1
}

// Raw 返回原始 SDK 客户端（用于高级用法）
func (s *ProfileLinkService) Raw() *identity.ProfileLinkClient {
	return s.client
}
