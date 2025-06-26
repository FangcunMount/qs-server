package patterns

import (
	"context"
	"fmt"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/shared/interfaces"
)

// BaseRepositoryOptions 基础仓储配置选项
type BaseRepositoryOptions struct {
	Name           string
	Timeout        time.Duration
	RetryCount     int
	RetryInterval  time.Duration
	MetricsEnabled bool
	TracingEnabled bool
}

// DefaultRepositoryOptions 默认仓储配置
func DefaultRepositoryOptions(name string) *BaseRepositoryOptions {
	return &BaseRepositoryOptions{
		Name:           name,
		Timeout:        30 * time.Second,
		RetryCount:     3,
		RetryInterval:  1 * time.Second,
		MetricsEnabled: true,
		TracingEnabled: true,
	}
}

// AggregateRepository 聚合根仓储接口
type AggregateRepository[T any, ID any] interface {
	interfaces.Repository[T, ID]

	// SaveAggregateWithEvents 保存聚合根并发布事件
	SaveAggregateWithEvents(ctx context.Context, aggregate T, events []interface{}) error
	// LoadAggregate 加载聚合根
	LoadAggregate(ctx context.Context, id ID) (T, error)
	// GetVersion 获取聚合根版本（用于乐观锁）
	GetVersion(ctx context.Context, id ID) (int, error)
}

// ReadOnlyRepository 只读仓储接口
type ReadOnlyRepository[T any, Q any] interface {
	interfaces.QueryRepository[T, Q]

	// GetByID 根据ID获取实体（只读）
	GetByID(ctx context.Context, id interface{}) (T, error)
	// List 获取实体列表（只读）
	List(ctx context.Context, query Q) ([]T, error)
	// Search 搜索实体（只读）
	Search(ctx context.Context, criteria interface{}) ([]T, error)
}

// WriteOnlyRepository 只写仓储接口
type WriteOnlyRepository[T any, ID any] interface {
	interfaces.Port

	// Create 创建实体
	Create(ctx context.Context, entity T) error
	// Update 更新实体
	Update(ctx context.Context, entity T) error
	// Delete 删除实体
	Delete(ctx context.Context, id ID) error
	// BulkCreate 批量创建
	BulkCreate(ctx context.Context, entities []T) error
	// BulkUpdate 批量更新
	BulkUpdate(ctx context.Context, entities []T) error
	// BulkDelete 批量删除
	BulkDelete(ctx context.Context, ids []ID) error
}

// CacheableRepository 支持缓存的仓储接口
type CacheableRepository[T any, ID any] interface {
	interfaces.Repository[T, ID]

	// GetFromCache 从缓存获取
	GetFromCache(ctx context.Context, id ID) (T, bool, error)
	// PutToCache 放入缓存
	PutToCache(ctx context.Context, id ID, entity T, ttl time.Duration) error
	// InvalidateCache 使缓存失效
	InvalidateCache(ctx context.Context, id ID) error
	// WarmUpCache 预热缓存
	WarmUpCache(ctx context.Context) error
}

// EventSourcingRepository 事件溯源仓储接口
type EventSourcingRepository[T any, ID any] interface {
	interfaces.Port

	// GetEvents 获取聚合根的所有事件
	GetEvents(ctx context.Context, aggregateID ID) ([]interface{}, error)
	// SaveEvents 保存事件
	SaveEvents(ctx context.Context, aggregateID ID, events []interface{}, expectedVersion int) error
	// GetSnapshot 获取快照
	GetSnapshot(ctx context.Context, aggregateID ID) (T, error)
	// SaveSnapshot 保存快照
	SaveSnapshot(ctx context.Context, aggregateID ID, snapshot T, version int) error
	// ReplayEvents 重放事件重建聚合根
	ReplayEvents(ctx context.Context, aggregateID ID) (T, error)
}

// RepositorySpec 仓储规约模式
type RepositorySpec[T any] interface {
	// IsSatisfiedBy 检查实体是否满足规约
	IsSatisfiedBy(entity T) bool
	// ToSQL 转换为SQL条件（如果支持）
	ToSQL() (string, []interface{}, error)
	// ToMongo 转换为MongoDB条件（如果支持）
	ToMongo() (interface{}, error)
}

// CompositeSpec 组合规约
type CompositeSpec[T any] struct {
	specs []RepositorySpec[T]
	logic SpecLogic
}

// SpecLogic 规约逻辑
type SpecLogic int

const (
	SpecAnd SpecLogic = iota
	SpecOr
	SpecNot
)

// NewCompositeSpec 创建组合规约
func NewCompositeSpec[T any](logic SpecLogic, specs ...RepositorySpec[T]) *CompositeSpec[T] {
	return &CompositeSpec[T]{
		specs: specs,
		logic: logic,
	}
}

// IsSatisfiedBy 实现规约接口
func (cs *CompositeSpec[T]) IsSatisfiedBy(entity T) bool {
	switch cs.logic {
	case SpecAnd:
		for _, spec := range cs.specs {
			if !spec.IsSatisfiedBy(entity) {
				return false
			}
		}
		return true
	case SpecOr:
		for _, spec := range cs.specs {
			if spec.IsSatisfiedBy(entity) {
				return true
			}
		}
		return false
	case SpecNot:
		if len(cs.specs) > 0 {
			return !cs.specs[0].IsSatisfiedBy(entity)
		}
		return true
	default:
		return false
	}
}

// RepositoryMiddleware 仓储中间件接口
type RepositoryMiddleware interface {
	// Name 中间件名称
	Name() string
	// Before 前置处理
	Before(ctx context.Context, operation string, args ...interface{}) (context.Context, error)
	// After 后置处理
	After(ctx context.Context, operation string, result interface{}, err error) error
}

// RepositoryDecorator 仓储装饰器
type RepositoryDecorator[T any, ID any] struct {
	repository  interfaces.Repository[T, ID]
	middlewares []RepositoryMiddleware
}

// NewRepositoryDecorator 创建仓储装饰器
func NewRepositoryDecorator[T any, ID any](repo interfaces.Repository[T, ID], middlewares ...RepositoryMiddleware) *RepositoryDecorator[T, ID] {
	return &RepositoryDecorator[T, ID]{
		repository:  repo,
		middlewares: middlewares,
	}
}

// Name 实现Port接口
func (rd *RepositoryDecorator[T, ID]) Name() string {
	return fmt.Sprintf("decorated-%s", rd.repository.Name())
}

// Close 实现Port接口
func (rd *RepositoryDecorator[T, ID]) Close(ctx context.Context) error {
	return rd.repository.Close(ctx)
}

// HealthCheck 实现Port接口
func (rd *RepositoryDecorator[T, ID]) HealthCheck(ctx context.Context) error {
	return rd.repository.HealthCheck(ctx)
}

// Save 实现Repository接口
func (rd *RepositoryDecorator[T, ID]) Save(ctx context.Context, entity T) error {
	_, err := rd.executeWithMiddleware(ctx, "Save", func(ctx context.Context) (interface{}, error) {
		return nil, rd.repository.Save(ctx, entity)
	}, entity)
	return err
}

// FindByID 实现Repository接口
func (rd *RepositoryDecorator[T, ID]) FindByID(ctx context.Context, id ID) (T, error) {
	result, err := rd.executeWithMiddleware(ctx, "FindByID", func(ctx context.Context) (interface{}, error) {
		return rd.repository.FindByID(ctx, id)
	}, id)

	if err != nil {
		var zero T
		return zero, err
	}

	return result.(T), nil
}

// Update 实现Repository接口
func (rd *RepositoryDecorator[T, ID]) Update(ctx context.Context, entity T) error {
	_, err := rd.executeWithMiddleware(ctx, "Update", func(ctx context.Context) (interface{}, error) {
		return nil, rd.repository.Update(ctx, entity)
	}, entity)
	return err
}

// Remove 实现Repository接口
func (rd *RepositoryDecorator[T, ID]) Remove(ctx context.Context, id ID) error {
	_, err := rd.executeWithMiddleware(ctx, "Remove", func(ctx context.Context) (interface{}, error) {
		return nil, rd.repository.Remove(ctx, id)
	}, id)
	return err
}

// Exists 实现Repository接口
func (rd *RepositoryDecorator[T, ID]) Exists(ctx context.Context, id ID) (bool, error) {
	result, err := rd.executeWithMiddleware(ctx, "Exists", func(ctx context.Context) (interface{}, error) {
		return rd.repository.Exists(ctx, id)
	}, id)

	if err != nil {
		return false, err
	}

	return result.(bool), nil
}

// executeWithMiddleware 执行中间件包装的操作
func (rd *RepositoryDecorator[T, ID]) executeWithMiddleware(
	ctx context.Context,
	operation string,
	fn func(context.Context) (interface{}, error),
	args ...interface{},
) (interface{}, error) {
	// 执行前置中间件
	for _, middleware := range rd.middlewares {
		var err error
		ctx, err = middleware.Before(ctx, operation, args...)
		if err != nil {
			return nil, fmt.Errorf("middleware %s before failed: %w", middleware.Name(), err)
		}
	}

	// 执行实际操作
	result, err := fn(ctx)

	// 执行后置中间件（逆序）
	for i := len(rd.middlewares) - 1; i >= 0; i-- {
		if middlewareErr := rd.middlewares[i].After(ctx, operation, result, err); middlewareErr != nil {
			// 记录中间件错误，但不影响原始结果
			// 这里可以记录日志
		}
	}

	return result, err
}

// RepositoryFactory 仓储工厂接口
type RepositoryFactory interface {
	// CreateRepository 创建仓储
	CreateRepository(repoType string, options *BaseRepositoryOptions) (interface{}, error)
	// RegisterRepository 注册仓储类型
	RegisterRepository(repoType string, creator func(*BaseRepositoryOptions) (interface{}, error))
	// GetSupportedTypes 获取支持的仓储类型
	GetSupportedTypes() []string
}

// DefaultRepositoryFactory 默认仓储工厂
type DefaultRepositoryFactory struct {
	creators map[string]func(*BaseRepositoryOptions) (interface{}, error)
}

// NewDefaultRepositoryFactory 创建默认仓储工厂
func NewDefaultRepositoryFactory() *DefaultRepositoryFactory {
	return &DefaultRepositoryFactory{
		creators: make(map[string]func(*BaseRepositoryOptions) (interface{}, error)),
	}
}

// CreateRepository 实现RepositoryFactory接口
func (f *DefaultRepositoryFactory) CreateRepository(repoType string, options *BaseRepositoryOptions) (interface{}, error) {
	creator, exists := f.creators[repoType]
	if !exists {
		return nil, fmt.Errorf("unsupported repository type: %s", repoType)
	}

	return creator(options)
}

// RegisterRepository 实现RepositoryFactory接口
func (f *DefaultRepositoryFactory) RegisterRepository(repoType string, creator func(*BaseRepositoryOptions) (interface{}, error)) {
	f.creators[repoType] = creator
}

// GetSupportedTypes 实现RepositoryFactory接口
func (f *DefaultRepositoryFactory) GetSupportedTypes() []string {
	types := make([]string, 0, len(f.creators))
	for repoType := range f.creators {
		types = append(types, repoType)
	}
	return types
}
