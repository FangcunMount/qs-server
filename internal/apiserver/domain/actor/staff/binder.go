package staff

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Binder Staff与用户绑定领域服务
// 负责 Staff 与 iam.User 的绑定和解绑
type Binder interface {
	// Bind 绑定用户
	// 场景：Staff创建后，需要绑定到具体的用户
	Bind(ctx context.Context, staff *Staff, userID int64) error

	// Unbind 解绑用户
	// 场景：用户离职或需要解除关联
	Unbind(ctx context.Context, staff *Staff) error

	// ValidateBinding 验证绑定关系的有效性
	// 场景：检查绑定的用户ID是否有效
	ValidateBinding(ctx context.Context, staff *Staff) error
}

// binder 绑定器实现
type binder struct {
	repo      Repository
	validator Validator
}

// NewBinder 创建绑定器
func NewBinder(repo Repository, validator Validator) Binder {
	return &binder{
		repo:      repo,
		validator: validator,
	}
}

// Bind 绑定用户
func (b *binder) Bind(ctx context.Context, staff *Staff, userID int64) error {
	// 1. 验证 userID
	if err := b.validator.ValidateUserID(userID); err != nil {
		return err
	}

	// 2. 检查是否已经绑定
	if staff.UserID() > 0 {
		return errors.WithCode(code.ErrValidation, "staff already bound to a user")
	}

	// 3. 检查该用户在该机构是否已经有绑定的员工
	existing, err := b.repo.FindByUser(ctx, staff.OrgID(), userID)
	if err == nil && existing != nil {
		return errors.WithCode(code.ErrUserAlreadyExists, "user already bound in this organization")
	}
	if !errors.IsCode(err, code.ErrUserNotFound) && err != nil {
		return err
	}

	// 4. 执行绑定
	staff.userID = userID

	return nil
}

// Unbind 解绑用户
func (b *binder) Unbind(ctx context.Context, staff *Staff) error {
	// 1. 检查是否已绑定
	if staff.UserID() <= 0 {
		return errors.WithCode(code.ErrValidation, "staff not bound to any user")
	}

	// 2. 业务规则：解绑时应该先清空所有角色
	if len(staff.Roles()) > 0 {
		return errors.WithCode(code.ErrValidation, "cannot unbind staff with roles, clear roles first")
	}

	// 3. 业务规则：解绑时应该先停用
	if staff.IsActive() {
		return errors.WithCode(code.ErrValidation, "cannot unbind active staff, deactivate first")
	}

	// 4. 执行解绑
	staff.userID = 0

	return nil
}

// ValidateBinding 验证绑定关系
func (b *binder) ValidateBinding(ctx context.Context, staff *Staff) error {
	// 检查是否有绑定
	if staff.UserID() <= 0 {
		return errors.WithCode(code.ErrValidation, "staff not bound to any user")
	}

	// 验证 userID 格式
	if err := b.validator.ValidateUserID(staff.UserID()); err != nil {
		return err
	}

	// TODO: 可以在这里添加调用用户服务验证用户是否存在的逻辑
	// userExists, err := userService.CheckUserExists(ctx, staff.UserID())
	// if err != nil || !userExists {
	//     return errors.WithCode(code.ErrUserNotFound, "bound user not found")
	// }

	return nil
}
