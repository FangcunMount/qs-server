package iam

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/log"
	identityv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/identity/v1"
	"github.com/FangcunMount/iam-contracts/pkg/sdk/identity"
)

// IdentityService 身份服务封装
// 提供用户信息查询功能
type IdentityService struct {
	client  *identity.Client
	enabled bool
}

// NewIdentityService 创建身份服务
func NewIdentityService(client *Client) (*IdentityService, error) {
	if client == nil || !client.enabled {
		return &IdentityService{enabled: false}, nil
	}

	sdkClient := client.SDK()
	if sdkClient == nil {
		return nil, fmt.Errorf("SDK client is nil")
	}

	identityClient := sdkClient.Identity()
	if identityClient == nil {
		return nil, fmt.Errorf("identity client is nil")
	}

	log.Info("IdentityService initialized")
	return &IdentityService{
		client:  identityClient,
		enabled: true,
	}, nil
}

// IsEnabled 检查服务是否启用
func (s *IdentityService) IsEnabled() bool {
	return s.enabled
}

// GetUser 获取用户信息
func (s *IdentityService) GetUser(ctx context.Context, userID string) (*identityv1.GetUserResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("identity service not enabled")
	}
	return s.client.GetUser(ctx, userID)
}

// BatchGetUsers 批量获取用户
func (s *IdentityService) BatchGetUsers(ctx context.Context, userIDs []string) (*identityv1.BatchGetUsersResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("identity service not enabled")
	}
	return s.client.BatchGetUsers(ctx, userIDs)
}

// SearchUsers 搜索用户
func (s *IdentityService) SearchUsers(ctx context.Context, req *identityv1.SearchUsersRequest) (*identityv1.SearchUsersResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("identity service not enabled")
	}
	return s.client.SearchUsers(ctx, req)
}

// NOTE: GetChild 和 BatchGetChildren 方法在 SDK identity.Client 中不存在
// 如果需要访问儿童信息，可以：
// 1. 通过 Guardianship 服务的 ListChildren 获取
// 2. 使用 Raw() 获取底层客户端，直接调用 gRPC 服务

// Raw 返回原始 SDK 客户端（用于高级用法）
func (s *IdentityService) Raw() *identity.Client {
	return s.client
}
