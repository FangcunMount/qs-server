package iam

import (
	"context"
	"fmt"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/logger"
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

	logger.L(context.Background()).Infow("IdentityService initialized",
		"component", "iam.identity",
		"result", "success",
	)
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

// LinkExternalIdentity 关联外部身份（SDK v0.0.5 新增）
// 用于将用户与第三方身份提供商关联
func (s *IdentityService) LinkExternalIdentity(ctx context.Context, req *identityv1.LinkExternalIdentityRequest) (*identityv1.LinkExternalIdentityResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("identity service not enabled")
	}
	return s.client.LinkExternalIdentity(ctx, req)
}

// Raw 返回原始 SDK 客户端（用于高级用法）
func (s *IdentityService) Raw() *identity.Client {
	return s.client
}

// CreateUser 创建用户的简化封装
// 返回新建用户的 numeric ID（int64）或错误
func (s *IdentityService) CreateUser(ctx context.Context, name, email, phone string) (int64, error) {
	if !s.enabled {
		return 0, fmt.Errorf("identity service not enabled")
	}
	// 构建 SDK 的 CreateUserRequest
	req := &identityv1.CreateUserRequest{
		Nickname: name,
		Email:    email,
		Phone:    phone,
	}

	resp, err := s.client.CreateUser(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("failed to create user in IAM: %w", err)
	}

	if resp == nil || resp.User == nil {
		return 0, fmt.Errorf("empty response or missing user from IAM CreateUser")
	}

	// SDK 返回的 User 对象的 ID 字段名为 `Id`（string），尝试解析为 int64
	uidStr := resp.User.Id
	if uidStr == "" {
		return 0, fmt.Errorf("empty user id returned from IAM")
	}
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse user id from IAM: %w", err)
	}
	return uid, nil
}
