package storage

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
)

// UserRepository 用户仓储端口
type UserRepository interface {
	// 基本 CRUD 操作
	Save(ctx context.Context, u *user.User) error
	FindByID(ctx context.Context, id user.UserID) (*user.User, error)
	FindByUsername(ctx context.Context, username string) (*user.User, error)
	FindByEmail(ctx context.Context, email string) (*user.User, error)
	Update(ctx context.Context, u *user.User) error
	Remove(ctx context.Context, id user.UserID) error

	// 业务查询
	FindActiveUsers(ctx context.Context) ([]*user.User, error)
	FindUsers(ctx context.Context, query UserQueryOptions) (*UserQueryResult, error)

	// 检查存在性
	ExistsByUsername(ctx context.Context, username string) (bool, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
}

// UserQueryOptions 用户查询选项
type UserQueryOptions struct {
	Offset    int
	Limit     int
	Keyword   *string
	Status    *user.Status
	SortBy    string
	SortOrder string
}

// UserQueryResult 用户查询结果
type UserQueryResult struct {
	Items      []*user.User
	TotalCount int64
	HasMore    bool
}
