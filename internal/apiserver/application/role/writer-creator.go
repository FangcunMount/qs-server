package role

import (
	"context"
	"fmt"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/role"
)

// WriterRepository 填写人仓储接口
type WriterRepository interface {
	Save(ctx context.Context, writer *role.Writer) error
	Update(ctx context.Context, writer *role.Writer) error
	FindByUserID(ctx context.Context, userID user.UserID) (*role.Writer, error)
	ExistsByUserID(ctx context.Context, userID user.UserID) bool
}

// WriterCreator 填写人创建器
// 职责：创建和管理填写人角色
type WriterCreator struct {
	writerRepo WriterRepository
}

// NewWriterCreator 创建填写人创建器
func NewWriterCreator(writerRepo WriterRepository) *WriterCreator {
	return &WriterCreator{
		writerRepo: writerRepo,
	}
}

// CreateWriter 创建填写人
func (c *WriterCreator) CreateWriter(
	ctx context.Context,
	userID user.UserID,
	name string,
) (*role.Writer, error) {
	// 检查是否已存在
	if c.writerRepo.ExistsByUserID(ctx, userID) {
		return nil, fmt.Errorf("writer already exists for user %v", userID)
	}

	// 创建填写人
	writer := role.NewWriter(userID, name)

	// 保存
	if err := c.writerRepo.Save(ctx, writer); err != nil {
		return nil, fmt.Errorf("failed to save writer: %w", err)
	}

	return writer, nil
}

// UpdateWriter 更新填写人信息
func (c *WriterCreator) UpdateWriter(
	ctx context.Context,
	userID user.UserID,
	name string,
) (*role.Writer, error) {
	// 查找填写人
	_, err := c.writerRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("writer not found: %w", err)
	}

	// 更新（重新创建值对象）
	writer := role.NewWriter(userID, name)

	// 保存更新
	if err := c.writerRepo.Update(ctx, writer); err != nil {
		return nil, fmt.Errorf("failed to update writer: %w", err)
	}

	return writer, nil
}

// GetWriterByUserID 根据用户ID获取填写人
func (c *WriterCreator) GetWriterByUserID(ctx context.Context, userID user.UserID) (*role.Writer, error) {
	return c.writerRepo.FindByUserID(ctx, userID)
}

// WriterExists 检查填写人是否存在
func (c *WriterCreator) WriterExists(ctx context.Context, userID user.UserID) bool {
	return c.writerRepo.ExistsByUserID(ctx, userID)
}
