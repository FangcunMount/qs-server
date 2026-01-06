package iam

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	identityv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/identity/v1"
	"github.com/FangcunMount/iam-contracts/pkg/sdk/identity"
)

// GuardianshipService 监护关系服务封装
// 提供监护关系查询和管理功能
type GuardianshipService struct {
	client  *identity.GuardianshipClient
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

	guardianshipClient := sdkClient.Guardianship()
	if guardianshipClient == nil {
		return nil, fmt.Errorf("guardianship client is nil")
	}

	logger.L(context.Background()).Infow("GuardianshipService initialized",
		"component", "iam.guardianship",
		"result", "success",
	)
	return &GuardianshipService{
		client:  guardianshipClient,
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

	ctx, release, err := acquire(ctx)
	if err != nil {
		return false, err
	}
	defer release()

	resp, err := s.client.IsGuardian(ctx, userID, childID)
	if err != nil {
		return false, err
	}

	return resp.IsGuardian, nil
}

// IsGuardianWithDetails 检查是否是监护人（返回详细信息）
func (s *GuardianshipService) IsGuardianWithDetails(ctx context.Context, userID, childID string) (*identityv1.IsGuardianResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.IsGuardian(ctx, userID, childID)
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

	// 通过查询监护人列表来验证 child 是否存在
	ctx, release, err := acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	resp, err := s.client.ListGuardians(ctx, &identityv1.ListGuardiansRequest{
		ChildId: childID,
	})
	if err != nil {
		logger.L(ctx).Errorw("Failed to validate child existence",
			"component", "iam.guardianship",
			"child_id", childID,
			"error", err.Error(),
		)
		return fmt.Errorf("failed to validate child existence in IAM: %w", err)
	}

	// 检查是否有返回结果（即使没有监护人，child 存在也应该返回空列表）
	if resp == nil {
		return fmt.Errorf("child %s does not exist in IAM system", childID)
	}

	logger.L(ctx).Debugw("Child validation passed",
		"component", "iam.guardianship",
		"child_id", childID,
		"guardians_count", len(resp.Items),
	)
	return nil
}

// ListChildren 列出用户的所有被监护儿童
func (s *GuardianshipService) ListChildren(ctx context.Context, userID string) (*identityv1.ListChildrenResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.ListChildren(ctx, &identityv1.ListChildrenRequest{
		UserId: userID,
	})
}

// ListGuardians 列出儿童的所有监护人
func (s *GuardianshipService) ListGuardians(ctx context.Context, childID string) (*identityv1.ListGuardiansResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.ListGuardians(ctx, &identityv1.ListGuardiansRequest{
		ChildId: childID,
	})
}

// AddGuardian 添加监护关系
func (s *GuardianshipService) AddGuardian(ctx context.Context, req *identityv1.AddGuardianRequest) (*identityv1.AddGuardianResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.AddGuardian(ctx, req)
}

// RevokeGuardian 撤销监护关系
func (s *GuardianshipService) RevokeGuardian(ctx context.Context, req *identityv1.RevokeGuardianRequest) (*identityv1.RevokeGuardianResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.RevokeGuardian(ctx, req)
}

// UpdateGuardianRelation 更新监护关系（SDK v0.0.5 新增）
func (s *GuardianshipService) UpdateGuardianRelation(ctx context.Context, req *identityv1.UpdateGuardianRelationRequest) (*identityv1.UpdateGuardianRelationResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.UpdateGuardianRelation(ctx, req)
}

// BatchRevokeGuardians 批量撤销监护关系（SDK v0.0.5 新增）
func (s *GuardianshipService) BatchRevokeGuardians(ctx context.Context, req *identityv1.BatchRevokeGuardiansRequest) (*identityv1.BatchRevokeGuardiansResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.BatchRevokeGuardians(ctx, req)
}

// ImportGuardians 批量导入监护关系（SDK v0.0.5 新增）
func (s *GuardianshipService) ImportGuardians(ctx context.Context, req *identityv1.ImportGuardiansRequest) (*identityv1.ImportGuardiansResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	ctx, release, err := acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.ImportGuardians(ctx, req)
}

// Raw 返回原始 SDK 客户端（用于高级用法）
func (s *GuardianshipService) Raw() *identity.GuardianshipClient {
	return s.client
}
