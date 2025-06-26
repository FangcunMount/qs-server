package interfaces

import "context"

// CommandHandler 命令处理器接口
type CommandHandler[TCommand any, TResult any] interface {
	Handle(ctx context.Context, command TCommand) (TResult, error)
}

// QueryHandler 查询处理器接口
type QueryHandler[TQuery any, TResult any] interface {
	Handle(ctx context.Context, query TQuery) (TResult, error)
}

// ApplicationService 应用服务基础接口
type ApplicationService interface {
	// 提供基础的应用服务标识
	ServiceName() string
}

// TransactionalService 支持事务的应用服务接口
type TransactionalService interface {
	ApplicationService
	// ExecuteInTransaction 在事务中执行操作
	ExecuteInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// ValidationService 验证服务接口
type ValidationService interface {
	// ValidateCommand 验证命令
	ValidateCommand(command interface{}) error
	// ValidateQuery 验证查询
	ValidateQuery(query interface{}) error
}

// EventPublisher 事件发布器接口
type EventPublisher interface {
	// Publish 发布领域事件
	Publish(ctx context.Context, events ...interface{}) error
}

// CacheService 缓存服务接口
type CacheService interface {
	// Get 获取缓存
	Get(ctx context.Context, key string, dest interface{}) error
	// Set 设置缓存
	Set(ctx context.Context, key string, value interface{}, ttl int) error
	// Delete 删除缓存
	Delete(ctx context.Context, key string) error
	// Clear 清空缓存
	Clear(ctx context.Context, pattern string) error
}

// PaginationRequest 分页请求
type PaginationRequest struct {
	Page     int `form:"page" json:"page"`
	PageSize int `form:"page_size" json:"page_size"`
}

// SetDefaults 设置默认值
func (p *PaginationRequest) SetDefaults() {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PageSize <= 0 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100 // 限制最大页面大小
	}
}

// GetOffset 计算偏移量
func (p *PaginationRequest) GetOffset() int {
	return (p.Page - 1) * p.PageSize
}

// PaginationResponse 分页响应
type PaginationResponse struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalCount int64 `json:"total_count"`
	TotalPages int   `json:"total_pages"`
	HasMore    bool  `json:"has_more"`
}

// NewPaginationResponse 创建分页响应
func NewPaginationResponse(req *PaginationRequest, totalCount int64) *PaginationResponse {
	totalPages := int((totalCount + int64(req.PageSize) - 1) / int64(req.PageSize))
	hasMore := req.Page < totalPages

	return &PaginationResponse{
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalCount: totalCount,
		TotalPages: totalPages,
		HasMore:    hasMore,
	}
}

// SortingRequest 排序请求
type SortingRequest struct {
	SortBy    string `form:"sort_by" json:"sort_by"`
	SortOrder string `form:"sort_order" json:"sort_order"` // asc, desc
}

// SetDefaults 设置默认值
func (s *SortingRequest) SetDefaults(defaultSortBy string) {
	if s.SortBy == "" {
		s.SortBy = defaultSortBy
	}
	if s.SortOrder == "" {
		s.SortOrder = "desc"
	}
}

// IsAscending 是否升序
func (s *SortingRequest) IsAscending() bool {
	return s.SortOrder == "asc"
}

// FilterRequest 过滤请求基础结构
type FilterRequest struct {
	Keyword *string `form:"keyword" json:"keyword"`
}

// HasKeyword 是否有关键字过滤
func (f *FilterRequest) HasKeyword() bool {
	return f.Keyword != nil && *f.Keyword != ""
}

// GetKeyword 获取关键字
func (f *FilterRequest) GetKeyword() string {
	if f.HasKeyword() {
		return *f.Keyword
	}
	return ""
}
