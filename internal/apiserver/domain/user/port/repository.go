package port

import (
	"context"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
)

// UserRepository 用户存储库接口（出站端口）
// 定义了与存储相关的所有操作契约
type UserRepository interface {
	// 基础 CRUD 操作
	Save(ctx context.Context, user *user.User) error
	FindByID(ctx context.Context, id user.UserID) (*user.User, error)
	Update(ctx context.Context, user *user.User) error
	Remove(ctx context.Context, id user.UserID) error

	// 查询操作
	FindByUsername(ctx context.Context, username string) (*user.User, error)
	FindByPhone(ctx context.Context, phone string) (*user.User, error)
	FindByEmail(ctx context.Context, email string) (*user.User, error)
	FindAll(ctx context.Context, limit, offset int) ([]*user.User, error)

	// 存在性检查
	ExistsByID(ctx context.Context, id user.UserID) bool
	ExistsByUsername(ctx context.Context, username string) bool
	ExistsByEmail(ctx context.Context, email string) bool
	ExistsByPhone(ctx context.Context, phone string) bool

	// 统计操作
	Count(ctx context.Context) (int64, error)
	CountByStatus(ctx context.Context, status user.Status) (int64, error)

	// 批量操作
	FindByIDs(ctx context.Context, ids []user.UserID) ([]*user.User, error)
	FindByStatus(ctx context.Context, status user.Status, limit, offset int) ([]*user.User, error)
}
