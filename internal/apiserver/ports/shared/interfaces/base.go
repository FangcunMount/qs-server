package interfaces

import (
	"context"
	"time"
)

// Port 端口基础接口
// 所有端口都应该实现这个接口，提供基本的生命周期管理
type Port interface {
	// Name 返回端口名称
	Name() string
	// Close 关闭端口，释放资源
	Close(ctx context.Context) error
	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error
}

// Repository 仓储模式基础接口
type Repository[T any, ID any] interface {
	Port

	// Save 保存实体
	Save(ctx context.Context, entity T) error
	// FindByID 根据ID查找实体
	FindByID(ctx context.Context, id ID) (T, error)
	// Update 更新实体
	Update(ctx context.Context, entity T) error
	// Remove 删除实体
	Remove(ctx context.Context, id ID) error
	// Exists 检查实体是否存在
	Exists(ctx context.Context, id ID) (bool, error)
}

// QueryRepository 查询仓储接口
type QueryRepository[T any, Q any] interface {
	Port

	// Find 查找实体列表
	Find(ctx context.Context, query Q) ([]T, error)
	// FindOne 查找单个实体
	FindOne(ctx context.Context, query Q) (T, error)
	// Count 统计数量
	Count(ctx context.Context, query Q) (int64, error)
}

// PaginatedQuery 分页查询接口
type PaginatedQuery interface {
	GetOffset() int
	GetLimit() int
	GetSortBy() string
	GetSortOrder() string
}

// PaginatedResult 分页结果接口
type PaginatedResult[T any] interface {
	GetItems() []T
	GetTotalCount() int64
	GetHasMore() bool
	GetPage() int
	GetPageSize() int
}

// CachePort 缓存端口接口
type CachePort interface {
	Port

	// Get 获取缓存值
	Get(ctx context.Context, key string, dest interface{}) error
	// Set 设置缓存值
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	// Delete 删除缓存
	Delete(ctx context.Context, key string) error
	// Clear 清空缓存
	Clear(ctx context.Context, pattern string) error
	// Exists 检查键是否存在
	Exists(ctx context.Context, key string) (bool, error)
}

// MessagePublisher 消息发布者接口
type MessagePublisher interface {
	Port

	// Publish 发布消息
	Publish(ctx context.Context, topic string, message interface{}) error
	// PublishAsync 异步发布消息
	PublishAsync(ctx context.Context, topic string, message interface{}) error
}

// MessageSubscriber 消息订阅者接口
type MessageSubscriber interface {
	Port

	// Subscribe 订阅消息
	Subscribe(ctx context.Context, topic string, handler MessageHandler) error
	// Unsubscribe 取消订阅
	Unsubscribe(ctx context.Context, topic string) error
}

// MessageHandler 消息处理器
type MessageHandler func(ctx context.Context, message interface{}) error

// EventPublisher 事件发布者接口
type EventPublisher interface {
	Port

	// PublishEvent 发布领域事件
	PublishEvent(ctx context.Context, event interface{}) error
	// PublishEvents 批量发布事件
	PublishEvents(ctx context.Context, events []interface{}) error
}

// ExternalServicePort 外部服务端口接口
type ExternalServicePort interface {
	Port

	// Call 调用外部服务
	Call(ctx context.Context, request interface{}) (interface{}, error)
	// CallAsync 异步调用外部服务
	CallAsync(ctx context.Context, request interface{}) error
}

// AuthenticationPort 认证端口接口
type AuthenticationPort interface {
	Port

	// Authenticate 认证用户
	Authenticate(ctx context.Context, credentials interface{}) (interface{}, error)
	// ValidateToken 验证令牌
	ValidateToken(ctx context.Context, token string) (interface{}, error)
	// RefreshToken 刷新令牌
	RefreshToken(ctx context.Context, refreshToken string) (interface{}, error)
}

// AuthorizationPort 授权端口接口
type AuthorizationPort interface {
	Port

	// Authorize 检查授权
	Authorize(ctx context.Context, subject, action, resource string) (bool, error)
	// GetPermissions 获取权限列表
	GetPermissions(ctx context.Context, subject string) ([]string, error)
}

// TransactionManager 事务管理器接口
type TransactionManager interface {
	Port

	// BeginTransaction 开始事务
	BeginTransaction(ctx context.Context) (Transaction, error)
	// ExecuteInTransaction 在事务中执行
	ExecuteInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// Transaction 事务接口
type Transaction interface {
	// Commit 提交事务
	Commit(ctx context.Context) error
	// Rollback 回滚事务
	Rollback(ctx context.Context) error
	// Context 获取事务上下文
	Context() context.Context
}

// UnitOfWork 工作单元接口
type UnitOfWork interface {
	Port

	// RegisterNew 注册新实体
	RegisterNew(entity interface{})
	// RegisterDirty 注册脏实体
	RegisterDirty(entity interface{})
	// RegisterDeleted 注册删除实体
	RegisterDeleted(entity interface{})
	// Commit 提交所有变更
	Commit(ctx context.Context) error
	// Rollback 回滚所有变更
	Rollback(ctx context.Context) error
}

// MetricsPort 度量端口接口
type MetricsPort interface {
	Port

	// Counter 计数器
	Counter(name string, labels map[string]string) Counter
	// Gauge 仪表盘
	Gauge(name string, labels map[string]string) Gauge
	// Histogram 直方图
	Histogram(name string, labels map[string]string) Histogram
}

// Counter 计数器接口
type Counter interface {
	Inc()
	Add(delta float64)
}

// Gauge 仪表盘接口
type Gauge interface {
	Set(value float64)
	Inc()
	Dec()
	Add(delta float64)
	Sub(delta float64)
}

// Histogram 直方图接口
type Histogram interface {
	Observe(value float64)
}

// LoggingPort 日志端口接口
type LoggingPort interface {
	Port

	// Debug 调试日志
	Debug(ctx context.Context, message string, fields map[string]interface{})
	// Info 信息日志
	Info(ctx context.Context, message string, fields map[string]interface{})
	// Warn 警告日志
	Warn(ctx context.Context, message string, fields map[string]interface{})
	// Error 错误日志
	Error(ctx context.Context, message string, err error, fields map[string]interface{})
}

// ConfigurationPort 配置端口接口
type ConfigurationPort interface {
	Port

	// Get 获取配置值
	Get(key string) (interface{}, error)
	// GetString 获取字符串配置
	GetString(key string) (string, error)
	// GetInt 获取整数配置
	GetInt(key string) (int, error)
	// GetBool 获取布尔配置
	GetBool(key string) (bool, error)
	// GetDuration 获取时间配置
	GetDuration(key string) (time.Duration, error)
	// Watch 监听配置变化
	Watch(key string, callback func(oldValue, newValue interface{})) error
}

// FileStoragePort 文件存储端口接口
type FileStoragePort interface {
	Port

	// Upload 上传文件
	Upload(ctx context.Context, path string, content []byte) error
	// Download 下载文件
	Download(ctx context.Context, path string) ([]byte, error)
	// Delete 删除文件
	Delete(ctx context.Context, path string) error
	// Exists 检查文件是否存在
	Exists(ctx context.Context, path string) (bool, error)
	// List 列出文件
	List(ctx context.Context, prefix string) ([]string, error)
}
