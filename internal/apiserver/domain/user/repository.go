package user

import "context"

// Repository 用户仓储接口
type Repository interface {
	// Save 保存用户
	Save(ctx context.Context, user *User) error

	// Update 更新用户
	Update(ctx context.Context, user *User) error

	// FindByID 根据ID查找用户
	FindByID(ctx context.Context, id UserID) (*User, error)

	// FindByUsername 根据用户名查找用户
	FindByUsername(ctx context.Context, username string) (*User, error)

	// FindByPhone 根据手机号查找用户
	FindByPhone(ctx context.Context, phone string) (*User, error)

	// FindByEmail 根据邮箱查找用户
	FindByEmail(ctx context.Context, email string) (*User, error)

	// Delete 删除用户
	Delete(ctx context.Context, id UserID) error

	// ExistsByUsername 检查用户名是否存在
	ExistsByUsername(ctx context.Context, username string) (bool, error)

	// ExistsByPhone 检查手机号是否存在
	ExistsByPhone(ctx context.Context, phone string) (bool, error)

	// ExistsByEmail 检查邮箱是否存在
	ExistsByEmail(ctx context.Context, email string) (bool, error)
}
