package staff

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// IAMSynchronizer IAM同步器领域服务
// 负责从 IAM 系统同步员工信息到本地
type IAMSynchronizer interface {
	// SyncBasicInfo 同步基本信息（姓名、邮箱、手机）
	SyncBasicInfo(ctx context.Context, staff *Staff, name, email, phone string) error

	// ValidateIAMBinding 验证 IAM 绑定的有效性
	// 例如检查 IAM 中的用户是否还存在、是否被停用等
	ValidateIAMBinding(ctx context.Context, staff *Staff) error
}

// iamSynchronizer IAM同步器实现
type iamSynchronizer struct {
	repo      Repository
	validator Validator
}

// NewIAMSynchronizer 创建IAM同步器
func NewIAMSynchronizer(repo Repository, validator Validator) IAMSynchronizer {
	return &iamSynchronizer{
		repo:      repo,
		validator: validator,
	}
}

// SyncBasicInfo 同步基本信息
func (s *iamSynchronizer) SyncBasicInfo(
	ctx context.Context,
	staff *Staff,
	name, email, phone string,
) error {
	// 验证参数
	if err := s.validator.ValidateName(name, false); err != nil {
		return err
	}
	if err := s.validator.ValidateEmail(email); err != nil {
		return err
	}
	if err := s.validator.ValidatePhone(phone); err != nil {
		return err
	}

	// 更新信息
	if name != "" {
		staff.name = name
	}
	staff.updateContactInfo(email, phone)

	return nil
}

// ValidateIAMBinding 验证IAM绑定
func (s *iamSynchronizer) ValidateIAMBinding(ctx context.Context, staff *Staff) error {
	// TODO: 这里可以调用 IAM 服务验证用户是否仍然存在
	// 例如：
	// _, err := iamClient.GetUser(ctx, staff.IAMUserID())
	// if err != nil {
	//     return errors.Wrap(err, "iam user not found or invalid")
	// }

	// 检查基本约束
	if staff.IAMUserID() <= 0 {
		return errors.WithCode(code.ErrValidation, "invalid iam user id")
	}

	return nil
}
