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
	// Bind 绑定到用户档案
	// 验证绑定的合法性，防止重复绑定或绑定冲突
	Bind(ctx context.Context, testee *Testee, profileID uint64) error

	// Unbind 解除档案绑定
	Unbind(ctx context.Context, testee *Testee) error

	// IsBound 检查是否已绑定
	IsBound(testee *Testee) bool
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

// Bind 绑定到用户档案
func (b *binder) Bind(ctx context.Context, testee *Testee, profileID uint64) error {
	if profileID == 0 {
		return errors.WithCode(code.ErrInvalidArgument, "invalid profile id")
	}

	// 检查是否已经绑定
	if testee.IsBoundToProfile() {
		currentProfileID := *testee.ProfileID()
		if currentProfileID == profileID {
			// 重复绑定同一个档案，幂等操作
			return nil
		}
		return errors.WithCode(code.ErrValidation, "testee already bound to another profile")
	}

	// 检查该档案是否已经被其他 Testee 绑定
	existingTestee, err := b.repo.FindByProfile(ctx, testee.orgID, profileID)
	if err == nil && existingTestee.ID() != testee.ID() {
		return errors.WithCode(code.ErrUserAlreadyExists, "profile already bound to another testee")
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
		// 幂等操作：未绑定时不报错
		return nil
	}

	// 解除绑定：将指针设为 nil
	testee.profileID = nil

	return nil
}

// IsBound 检查是否已绑定
func (b *binder) IsBound(testee *Testee) bool {
	return testee.IsBoundToProfile()
}
