package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// BaseRepository 基础仓储适配器，提供通用的数据库操作
type BaseRepository struct {
	db *gorm.DB
}

// NewBaseRepository 创建基础仓储适配器
func NewBaseRepository(db *gorm.DB) *BaseRepository {
	return &BaseRepository{db: db}
}

// DB 获取数据库连接
func (r *BaseRepository) DB() *gorm.DB {
	return r.db
}

// WithContext 带上下文的数据库连接
func (r *BaseRepository) WithContext(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// Create 创建记录
func (r *BaseRepository) Create(ctx context.Context, model interface{}) error {
	return r.db.WithContext(ctx).Create(model).Error
}

// Save 保存记录（创建或更新）
func (r *BaseRepository) Save(ctx context.Context, model interface{}) error {
	return r.db.WithContext(ctx).Save(model).Error
}

// FindByID 根据ID查找记录
func (r *BaseRepository) FindByID(ctx context.Context, model interface{}, id interface{}) error {
	err := r.db.WithContext(ctx).Where("id = ?", id).First(model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil // 返回nil表示记录不存在，上层判断
	}
	return err
}

// FindByField 根据字段查找记录
func (r *BaseRepository) FindByField(ctx context.Context, model interface{}, field string, value interface{}) error {
	err := r.db.WithContext(ctx).Where(field+" = ?", value).First(model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

// Update 更新记录
func (r *BaseRepository) Update(ctx context.Context, model interface{}) error {
	return r.db.WithContext(ctx).Save(model).Error
}

// Delete 删除记录
func (r *BaseRepository) Delete(ctx context.Context, model interface{}) error {
	return r.db.WithContext(ctx).Delete(model).Error
}

// DeleteByID 根据ID删除记录
func (r *BaseRepository) DeleteByID(ctx context.Context, model interface{}, id interface{}) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(model).Error
}

// ExistsByID 检查ID是否存在
func (r *BaseRepository) ExistsByID(ctx context.Context, model interface{}, id interface{}) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(model).Where("id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ExistsByField 检查字段值是否存在
func (r *BaseRepository) ExistsByField(ctx context.Context, model interface{}, field string, value interface{}) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(model).Where(field+" = ?", value).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// FindWithConditions 根据条件查找记录
func (r *BaseRepository) FindWithConditions(ctx context.Context, models interface{}, conditions map[string]interface{}) error {
	db := r.db.WithContext(ctx)
	for field, value := range conditions {
		db = db.Where(field+" = ?", value)
	}
	return db.Find(models).Error
}

// CountWithConditions 根据条件统计记录数
func (r *BaseRepository) CountWithConditions(ctx context.Context, model interface{}, conditions map[string]interface{}) (int64, error) {
	var count int64
	db := r.db.WithContext(ctx).Model(model)
	for field, value := range conditions {
		db = db.Where(field+" = ?", value)
	}
	if err := db.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Transaction 执行事务
func (r *BaseRepository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// PaginatedQuery 分页查询构建器
type PaginatedQuery struct {
	db       *gorm.DB
	model    interface{}
	offset   int
	limit    int
	orderBy  string
	keywords []string
	fields   []string
}

// NewPaginatedQuery 创建分页查询构建器
func (r *BaseRepository) NewPaginatedQuery(ctx context.Context, model interface{}) *PaginatedQuery {
	return &PaginatedQuery{
		db:    r.db.WithContext(ctx).Model(model),
		model: model,
	}
}

// Offset 设置偏移量
func (q *PaginatedQuery) Offset(offset int) *PaginatedQuery {
	q.offset = offset
	return q
}

// Limit 设置限制数量
func (q *PaginatedQuery) Limit(limit int) *PaginatedQuery {
	q.limit = limit
	return q
}

// OrderBy 设置排序
func (q *PaginatedQuery) OrderBy(orderBy string) *PaginatedQuery {
	q.orderBy = orderBy
	return q
}

// Where 添加条件
func (q *PaginatedQuery) Where(query interface{}, args ...interface{}) *PaginatedQuery {
	q.db = q.db.Where(query, args...)
	return q
}

// Search 添加搜索条件（在指定字段中搜索关键词）
func (q *PaginatedQuery) Search(keyword string, fields ...string) *PaginatedQuery {
	if keyword != "" && len(fields) > 0 {
		conditions := make([]string, len(fields))
		args := make([]interface{}, len(fields))
		for i, field := range fields {
			conditions[i] = field + " LIKE ?"
			args[i] = "%" + keyword + "%"
		}

		// 构建OR条件
		orCondition := "(" + conditions[0]
		for i := 1; i < len(conditions); i++ {
			orCondition += " OR " + conditions[i]
		}
		orCondition += ")"

		q.db = q.db.Where(orCondition, args...)
	}
	return q
}

// PaginatedResult 分页查询结果
type PaginatedResult struct {
	Items      interface{} `json:"items"`
	TotalCount int64       `json:"total_count"`
	HasMore    bool        `json:"has_more"`
}

// Execute 执行分页查询
func (q *PaginatedQuery) Execute(results interface{}) (*PaginatedResult, error) {
	// 获取总数
	var totalCount int64
	if err := q.db.Count(&totalCount).Error; err != nil {
		return nil, err
	}

	// 应用排序
	if q.orderBy != "" {
		q.db = q.db.Order(q.orderBy)
	} else {
		q.db = q.db.Order("created_at DESC")
	}

	// 应用分页
	if err := q.db.Offset(q.offset).Limit(q.limit).Find(results).Error; err != nil {
		return nil, err
	}

	// 计算是否还有更多数据
	hasMore := int64(q.offset+q.limit) < totalCount

	return &PaginatedResult{
		Items:      results,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}
