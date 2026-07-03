// Package iamport 为 collection application 提供 IAM 端口，隔离 infra 依赖。
package iamport

import (
	"context"

	collectioniam "github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
)

type (
	CreateProfileInput  = collectioniam.CreateProfileInput
	CreateProfileResult = collectioniam.CreateProfileResult
)

// ProfileCreator 创建 IAM 档案。
type ProfileCreator interface {
	IsEnabled() bool
	CreateProfile(ctx context.Context, input CreateProfileInput) (*CreateProfileResult, error)
}

// ProfileLinkChecker 校验档案关联（答卷提交等场景）。
type ProfileLinkChecker interface {
	IsEnabled() bool
	GetDefaultOrgID() uint64
	HasActiveProfileLink(ctx context.Context, userID, profileID string) (bool, error)
}

// OrgDefaults 读取默认组织配置。
type OrgDefaults interface {
	GetDefaultOrgID() uint64
}
