package testee

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Binder IAM 绑定器领域服务
// 负责 Testee 与 IAM 系统（User/Child）的绑定关系管理
type Binder interface {
	// BindToIAMUser 绑定到 IAM 用户
	// 验证绑定的合法性，防止重复绑定或绑定冲突
	BindToIAMUser(ctx context.Context, testee *Testee, iamUserID int64) error

	// BindToIAMChild 绑定到 IAM 儿童档案
	BindToIAMChild(ctx context.Context, testee *Testee, iamChildID int64) error

	// Unbind 解除 IAM 绑定
	Unbind(ctx context.Context, testee *Testee) error

	// VerifyBinding 验证绑定关系的有效性
	// 例如检查 IAM 中的用户/儿童是否还存在
	VerifyBinding(ctx context.Context, testee *Testee) error
}

// binder 绑定器实现
type binder struct {
	repo Repository
}

// NewBinder 创建绑定器
func NewBinder(repo Repository) Binder {
	return &binder{
		repo: repo,
	}
}

// BindToIAMUser 绑定到 IAM 用户
func (b *binder) BindToIAMUser(ctx context.Context, testee *Testee, iamUserID int64) error {
	if iamUserID <= 0 {
		return errors.WithCode(code.ErrInvalidArgument, "invalid iam user id")
	}

	// 检查是否已经绑定了其他身份
	if testee.iamChildID != nil {
		return errors.WithCode(code.ErrValidation, "testee already bound to iam child, cannot bind to user")
	}

	// 检查该 IAM User 是否已经被其他 Testee 绑定
	existingTestee, err := b.repo.FindByIAMUser(ctx, testee.orgID, iamUserID)
	if err == nil && existingTestee.ID() != testee.ID() {
		return errors.WithCode(code.ErrValidation, "iam user already bound to another testee")
	}
	if err != nil && !errors.IsCode(err, code.ErrUserNotFound) {
		return errors.Wrap(err, "failed to check iam user binding")
	}

	// 执行绑定
	testee.bindIAMUser(iamUserID)

	return nil
}

// BindToIAMChild 绑定到 IAM 儿童档案
func (b *binder) BindToIAMChild(ctx context.Context, testee *Testee, iamChildID int64) error {
	if iamChildID <= 0 {
		return errors.WithCode(code.ErrInvalidArgument, "invalid iam child id")
	}

	// 检查是否已经绑定了其他身份
	if testee.iamUserID != nil {
		return errors.WithCode(code.ErrValidation, "testee already bound to iam user, cannot bind to child")
	}

	// 检查该 IAM Child 是否已经被其他 Testee 绑定
	existingTestee, err := b.repo.FindByIAMChild(ctx, testee.orgID, iamChildID)
	if err == nil && existingTestee.ID() != testee.ID() {
		return errors.WithCode(code.ErrValidation, "iam child already bound to another testee")
	}
	if err != nil && !errors.IsCode(err, code.ErrUserNotFound) {
		return errors.Wrap(err, "failed to check iam child binding")
	}

	// 执行绑定
	testee.bindIAMChild(iamChildID)

	return nil
}

// Unbind 解除绑定
func (b *binder) Unbind(ctx context.Context, testee *Testee) error {
	if !testee.IsBoundToIAM() {
		return errors.WithCode(code.ErrValidation, "testee is not bound to any iam identity")
	}

	// 解除绑定：将指针设为 nil
	testee.iamUserID = nil
	testee.iamChildID = nil

	return nil
}

// VerifyBinding 验证绑定的有效性
func (b *binder) VerifyBinding(ctx context.Context, testee *Testee) error {
	if !testee.IsBoundToIAM() {
		return nil // 未绑定时无需验证
	}

	// TODO: 这里可以调用 IAM 服务验证用户/儿童是否仍然存在
	// 例如：
	// if testee.iamUserID != nil {
	//     _, err := iamClient.GetUser(ctx, *testee.iamUserID)
	//     if err != nil {
	//         return errors.Wrap(err, "iam user not found or invalid")
	//     }
	// }

	return nil
}
