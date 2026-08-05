package iam

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	identityv2 "github.com/FangcunMount/iam/v2/api/grpc/iam/identity/v2"
	"github.com/FangcunMount/iam/v2/pkg/sdk/identity"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
)

// IdentityService 身份服务封装
// 提供用户信息查询功能
type IdentityService struct {
	client  *identity.Client
	enabled bool
	limiter backpressure.Acquirer
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
		limiter: client.Limiter(),
	}, nil
}

// IsEnabled 检查服务是否启用
func (s *IdentityService) IsEnabled() bool {
	return s.enabled
}

// GetUser 获取用户信息
func (s *IdentityService) GetUser(ctx context.Context, userID string) (*identityv2.GetUserResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("identity service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.GetUser(ctx, userID)
}

// BatchGetUsers 批量获取用户
func (s *IdentityService) BatchGetUsers(ctx context.Context, userIDs []string) (*identityv2.BatchGetUsersResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("identity service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.BatchGetUsers(ctx, userIDs)
}

// SearchUsers 搜索用户
func (s *IdentityService) SearchUsers(ctx context.Context, req *identityv2.SearchUsersRequest) (*identityv2.SearchUsersResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("identity service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.SearchUsers(ctx, req)
}

// Raw 返回原始 SDK 客户端（用于高级用法）
func (s *IdentityService) Raw() *identity.Client {
	return s.client
}

func (s *IdentityService) acquire(ctx context.Context) (context.Context, func(), error) {
	if s == nil || s.limiter == nil {
		return ctx, func() {}, nil
	}
	return s.limiter.Acquire(ctx)
}
