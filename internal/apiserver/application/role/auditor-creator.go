package role

import (
	"context"
	"fmt"
	"time"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/role"
)

// AuditorRepository 审核员仓储接口
type AuditorRepository interface {
	Save(ctx context.Context, auditor *role.Auditor) error
	Update(ctx context.Context, auditor *role.Auditor) error
	FindByUserID(ctx context.Context, userID user.UserID) (*role.Auditor, error)
	FindByEmployeeID(ctx context.Context, employeeID string) (*role.Auditor, error)
	ExistsByUserID(ctx context.Context, userID user.UserID) bool
	ExistsByEmployeeID(ctx context.Context, employeeID string) bool
}

// AuditorCreator 审核员创建器
// 职责：创建和管理审核员角色
type AuditorCreator struct {
	auditorRepo AuditorRepository
}

// NewAuditorCreator 创建审核员创建器
func NewAuditorCreator(auditorRepo AuditorRepository) *AuditorCreator {
	return &AuditorCreator{
		auditorRepo: auditorRepo,
	}
}

// CreateAuditor 创建审核员
// 用于创建审核账户时创建 Auditor
func (c *AuditorCreator) CreateAuditor(
	ctx context.Context,
	userID user.UserID,
	name string,
	employeeID string,
	department string,
	position string,
	hiredAt *time.Time,
) (*role.Auditor, error) {
	// 检查用户是否已有审核员角色
	if c.auditorRepo.ExistsByUserID(ctx, userID) {
		return nil, fmt.Errorf("auditor already exists for user %v", userID)
	}

	// 检查员工编号是否已存在
	if c.auditorRepo.ExistsByEmployeeID(ctx, employeeID) {
		return nil, fmt.Errorf("employee ID %s already exists", employeeID)
	}

	// 创建审核员
	auditor := role.NewAuditor(userID, name, employeeID)

	// 设置可选属性
	if department != "" {
		auditor.WithDepartment(department)
	}
	if position != "" {
		auditor.WithPosition(position)
	}
	if hiredAt != nil && !hiredAt.IsZero() {
		auditor.WithHiredAt(*hiredAt)
	}

	// 保存
	if err := c.auditorRepo.Save(ctx, auditor); err != nil {
		return nil, fmt.Errorf("failed to save auditor: %w", err)
	}

	return auditor, nil
}

// UpdateAuditorInfo 更新审核员基本信息
func (c *AuditorCreator) UpdateAuditorInfo(
	ctx context.Context,
	userID user.UserID,
	name *string,
	department *string,
	position *string,
) (*role.Auditor, error) {
	// 查找审核员
	auditor, err := c.auditorRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("auditor not found: %w", err)
	}

	// 更新属性（需要重新创建，因为 Auditor 是值对象）
	if name != nil && *name != "" {
		auditor = role.NewAuditor(userID, *name, auditor.GetEmployeeID())
		auditor.WithDepartment(auditor.GetDepartment())
		auditor.WithPosition(auditor.GetPosition())
		auditor.WithStatus(auditor.GetStatus())
		auditor.WithHiredAt(auditor.GetHiredAt())
	}
	if department != nil {
		auditor.WithDepartment(*department)
	}
	if position != nil {
		auditor.WithPosition(*position)
	}

	// 保存更新
	if err := c.auditorRepo.Update(ctx, auditor); err != nil {
		return nil, fmt.Errorf("failed to update auditor: %w", err)
	}

	return auditor, nil
}

// UpdateAuditorStatus 更新审核员状态
func (c *AuditorCreator) UpdateAuditorStatus(
	ctx context.Context,
	userID user.UserID,
	status role.Status,
) (*role.Auditor, error) {
	// 查找审核员
	auditor, err := c.auditorRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("auditor not found: %w", err)
	}

	// 更新状态
	auditor.WithStatus(status)

	// 保存更新
	if err := c.auditorRepo.Update(ctx, auditor); err != nil {
		return nil, fmt.Errorf("failed to update auditor status: %w", err)
	}

	return auditor, nil
}

// ActivateAuditor 激活审核员（设置为在职状态）
func (c *AuditorCreator) ActivateAuditor(ctx context.Context, userID user.UserID) error {
	_, err := c.UpdateAuditorStatus(ctx, userID, role.StatusOnDuty)
	return err
}

// SuspendAuditor 停职审核员
func (c *AuditorCreator) SuspendAuditor(ctx context.Context, userID user.UserID) error {
	_, err := c.UpdateAuditorStatus(ctx, userID, role.StatusSuspended)
	return err
}

// ResignAuditor 离职审核员
func (c *AuditorCreator) ResignAuditor(ctx context.Context, userID user.UserID) error {
	_, err := c.UpdateAuditorStatus(ctx, userID, role.StatusResigned)
	return err
}

// SetAuditorOnLeave 设置审核员为休假状态
func (c *AuditorCreator) SetAuditorOnLeave(ctx context.Context, userID user.UserID) error {
	_, err := c.UpdateAuditorStatus(ctx, userID, role.StatusOnLeave)
	return err
}

// GetAuditorByUserID 根据用户ID获取审核员
func (c *AuditorCreator) GetAuditorByUserID(ctx context.Context, userID user.UserID) (*role.Auditor, error) {
	return c.auditorRepo.FindByUserID(ctx, userID)
}

// GetAuditorByEmployeeID 根据员工编号获取审核员
func (c *AuditorCreator) GetAuditorByEmployeeID(ctx context.Context, employeeID string) (*role.Auditor, error) {
	return c.auditorRepo.FindByEmployeeID(ctx, employeeID)
}

// AuditorExists 检查审核员是否存在
func (c *AuditorCreator) AuditorExists(ctx context.Context, userID user.UserID) bool {
	return c.auditorRepo.ExistsByUserID(ctx, userID)
}

// CanAudit 检查审核员是否可以审核
func (c *AuditorCreator) CanAudit(ctx context.Context, userID user.UserID) (bool, error) {
	auditor, err := c.auditorRepo.FindByUserID(ctx, userID)
	if err != nil {
		return false, err
	}
	return auditor.CanAudit(), nil
}
