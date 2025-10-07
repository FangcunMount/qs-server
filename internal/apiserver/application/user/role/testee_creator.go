package role

import (
	"context"
	"fmt"
	"time"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/role"
)

// TesteeRepository 受试者仓储接口
type TesteeRepository interface {
	Save(ctx context.Context, testee *role.Testee) error
	Update(ctx context.Context, testee *role.Testee) error
	FindByUserID(ctx context.Context, userID user.UserID) (*role.Testee, error)
	ExistsByUserID(ctx context.Context, userID user.UserID) bool
}

// TesteeCreator 受试者创建器
// 职责：创建和管理受试者角色
type TesteeCreator struct {
	testeeRepo TesteeRepository
}

// NewTesteeCreator 创建受试者创建器
func NewTesteeCreator(testeeRepo TesteeRepository) *TesteeCreator {
	return &TesteeCreator{
		testeeRepo: testeeRepo,
	}
}

// CreateTestee 创建受试者
// 用于用户在小程序侧注册受试者时创建 Testee
func (c *TesteeCreator) CreateTestee(
	ctx context.Context,
	userID user.UserID,
	name string,
	sex uint8,
	birthday *time.Time,
) (*role.Testee, error) {
	// 检查是否已存在
	if c.testeeRepo.ExistsByUserID(ctx, userID) {
		return nil, fmt.Errorf("testee already exists for user %v", userID)
	}

	// 创建受试者
	testee := role.NewTestee(userID, name)

	// 设置可选属性
	if sex > 0 {
		testee.WithSex(sex)
	}
	if birthday != nil && !birthday.IsZero() {
		testee.WithBirthday(*birthday)
	}

	// 保存
	if err := c.testeeRepo.Save(ctx, testee); err != nil {
		return nil, fmt.Errorf("failed to save testee: %w", err)
	}

	return testee, nil
}

// UpdateTestee 更新受试者信息
func (c *TesteeCreator) UpdateTestee(
	ctx context.Context,
	userID user.UserID,
	name *string,
	sex *uint8,
	birthday *time.Time,
) (*role.Testee, error) {
	// 查找受试者
	testee, err := c.testeeRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("testee not found: %w", err)
	}

	// 更新属性
	if name != nil && *name != "" {
		testee = role.NewTestee(userID, *name)
		testee.WithSex(testee.GetSex())
		testee.WithBirthday(testee.GetBirthday())
	}
	if sex != nil {
		testee.WithSex(*sex)
	}
	if birthday != nil && !birthday.IsZero() {
		testee.WithBirthday(*birthday)
	}

	// 保存更新
	if err := c.testeeRepo.Update(ctx, testee); err != nil {
		return nil, fmt.Errorf("failed to update testee: %w", err)
	}

	return testee, nil
}

// GetTesteeByUserID 根据用户ID获取受试者
func (c *TesteeCreator) GetTesteeByUserID(ctx context.Context, userID user.UserID) (*role.Testee, error) {
	return c.testeeRepo.FindByUserID(ctx, userID)
}

// TesteeExists 检查受试者是否存在
func (c *TesteeCreator) TesteeExists(ctx context.Context, userID user.UserID) bool {
	return c.testeeRepo.ExistsByUserID(ctx, userID)
}
