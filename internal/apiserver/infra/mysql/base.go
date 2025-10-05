package mysql

import (
	"context"

	"gorm.io/gorm"
)

// 泛型结构体，支持任意实现了 Syncable 的实体类型
type BaseRepository[T Syncable] struct {
	db *gorm.DB
}

func NewBaseRepository[T Syncable](db *gorm.DB) BaseRepository[T] {
	return BaseRepository[T]{db: db}
}

// DB 获取数据库连接
func (r *BaseRepository[T]) DB() *gorm.DB {
	return r.db
}

// WithContext 带上下文的数据库连接
func (r *BaseRepository[T]) WithContext(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// CreateAndSync 将实体插入数据库，并通过回调函数同步字段回 domain 层
func (r *BaseRepository[T]) CreateAndSync(ctx context.Context, entity T, sync func(T)) error {
	result := r.db.WithContext(ctx).Create(entity)
	if result.Error != nil {
		return result.Error
	}
	sync(entity)
	return nil
}

// UpdateAndSync 更新实体并同步时间戳等字段
func (r *BaseRepository[T]) UpdateAndSync(ctx context.Context, entity T, sync func(T)) error {
	result := r.db.WithContext(ctx).Updates(entity)
	if result.Error != nil {
		return result.Error
	}
	sync(entity)
	return nil
}

// FindByID 根据 ID 查询实体
func (r *BaseRepository[T]) FindByID(ctx context.Context, id uint64) (T, error) {
	var entity T
	result := r.db.WithContext(ctx).First(&entity, id)
	if result.Error != nil {
		var zero T
		return zero, result.Error
	}
	return entity, nil
}

// FindByField 根据字段查找记录
func (r *BaseRepository[T]) FindByField(ctx context.Context, model interface{}, field string, value interface{}) error {
	err := r.db.WithContext(ctx).Where(field+" = ?", value).First(model).Error
	return err // 直接返回错误，包括 gorm.ErrRecordNotFound
}

// DeleteByID 根据 ID 删除实体
func (r *BaseRepository[T]) DeleteByID(ctx context.Context, id uint64) error {
	var entity T
	result := r.db.WithContext(ctx).Delete(&entity, id)
	return result.Error
}

// ExistsByID 判断是否存在指定 ID 的记录
func (r *BaseRepository[T]) ExistsByID(ctx context.Context, id uint64) (bool, error) {
	var count int64
	var entity T
	result := r.db.WithContext(ctx).Model(&entity).Where("id = ?", id).Count(&count)
	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

// ExistsByField 检查字段值是否存在
func (r *BaseRepository[T]) ExistsByField(ctx context.Context, model interface{}, field string, value interface{}) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(model).Where(field+" = ?", value).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// FindWithConditions 根据条件查找记录
func (r *BaseRepository[T]) FindWithConditions(ctx context.Context, models interface{}, conditions map[string]interface{}) ([]T, error) {
	db := r.db.WithContext(ctx)
	for field, value := range conditions {
		db = db.Where(field+" = ?", value)
	}
	var entities []T
	if err := db.Find(&entities).Error; err != nil {
		return nil, err
	}
	return entities, nil
}

// FindList 查询列表
func (r *BaseRepository[T]) FindList(ctx context.Context, models interface{}, conditions map[string]string, page, pageSize int) ([]T, error) {
	db := r.db.WithContext(ctx)
	for field, value := range conditions {
		db = db.Where(field+" = ?", value)
	}
	if page > 0 {
		db = db.Offset((page - 1) * pageSize)
	}
	if pageSize > 0 {
		db = db.Limit(pageSize)
	}
	if err := db.Find(models).Error; err != nil {
		return nil, err
	}
	entities := make([]T, 0)
	if err := db.Find(&entities).Error; err != nil {
		return nil, err
	}
	return entities, nil
}

// CountWithConditions 根据条件统计记录数
func (r *BaseRepository[T]) CountWithConditions(ctx context.Context, model interface{}, conditions map[string]string) (int64, error) {
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
