package testee

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Binder 档案绑定器领域服务
// 负责 Testee 与用户档案（Profile）的绑定关系管理
// 注意：当前 Profile 对应 IAM.Child，未来可重构为更通用的档案系统
type Binder interface {
	// BindToProfile 绑定到用户档案
	// 验证绑定的合法性，防止重复绑定或绑定冲突
	BindToProfile(ctx context.Context, testee *Testee, profileID uint64) error

	// Unbind 解除档案绑定
	Unbind(ctx context.Context, testee *Testee) error

	// VerifyBinding 验证绑定关系的有效性
	// 例如检查档案是否还存在
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

// BindToProfile 绑定到用户档案
func (b *binder) BindToProfile(ctx context.Context, testee *Testee, profileID uint64) error {
	if profileID == 0 {
		return errors.WithCode(code.ErrInvalidArgument, "invalid profile id")
	}

	// 检查是否已经绑定
	if testee.IsBoundToProfile() {
		return errors.WithCode(code.ErrValidation, "testee already bound to a profile")
	}

	// 检查该档案是否已经被其他 Testee 绑定
	existingTestee, err := b.repo.FindByProfile(ctx, testee.orgID, profileID)
	if err == nil && existingTestee.ID() != testee.ID() {
		return errors.WithCode(code.ErrValidation, "profile already bound to another testee")
	}
	if err != nil && !errors.IsCode(err, code.ErrUserNotFound) {
		return errors.Wrap(err, "failed to check profile binding")
	}

	// 执行绑定
	testee.bindProfile(profileID)

	return nil
}

// Unbind 解除绑定
func (b *binder) Unbind(ctx context.Context, testee *Testee) error {
	if !testee.IsBoundToProfile() {
		return errors.WithCode(code.ErrValidation, "testee is not bound to any profile")
	}

	// 解除绑定：将指针设为 nil
	testee.profileID = nil

	return nil
}

// VerifyBinding 验证绑定的有效性
func (b *binder) VerifyBinding(ctx context.Context, testee *Testee) error {
	if !testee.IsBoundToProfile() {
		return nil // 未绑定时无需验证
	}

	// TODO: 这里可以调用档案服务验证档案是否仍然存在
	// 例如：
	// if testee.profileID != nil {
	//     _, err := profileClient.GetProfile(ctx, *testee.profileID)
	//     if err != nil {
	//         return errors.Wrap(err, "profile not found or invalid")
	//     }
	// }

	return nil
}
