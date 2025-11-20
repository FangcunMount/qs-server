package mysql

import (
	"context"

	"gorm.io/gorm"
)

// BaseRepository provides common CRUD helpers for GORM repositories.
type BaseRepository[T Syncable] struct {
	db *gorm.DB
	// errTranslator transforms DB-level errors into domain/business errors.
	// If nil, no translation is performed.
	errTranslator func(error) error
}

// NewBaseRepository constructs a repository wrapper for the provided DB.
func NewBaseRepository[T Syncable](db *gorm.DB) BaseRepository[T] {
	return BaseRepository[T]{db: db}
}

// SetErrorTranslator registers a function to translate DB errors into
// domain/business errors. This allows repositories to map driver-specific
// messages (unique constraint, duplicate entry) to structured errors.
func (r *BaseRepository[T]) SetErrorTranslator(f func(error) error) {
	r.errTranslator = f
}

// DB exposes the underlying *gorm.DB for advanced usages.
func (r *BaseRepository[T]) DB() *gorm.DB {
	return r.db
}

// WithContext attaches a context to the DB handle.
func (r *BaseRepository[T]) WithContext(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// CreateAndSync persists an entity and lets the caller sync generated fields back.
func (r *BaseRepository[T]) CreateAndSync(ctx context.Context, entity T, sync func(T)) error {
	result := r.db.WithContext(ctx).Create(entity)
	if result.Error != nil {
		if r.errTranslator != nil {
			return r.errTranslator(result.Error)
		}
		return result.Error
	}
	sync(entity)
	return nil
}

// UpdateAndSync updates an entity and triggers the sync callback.
func (r *BaseRepository[T]) UpdateAndSync(ctx context.Context, entity T, sync func(T)) error {
	result := r.db.WithContext(ctx).Updates(entity)
	if result.Error != nil {
		if r.errTranslator != nil {
			return r.errTranslator(result.Error)
		}
		return result.Error
	}
	sync(entity)
	return nil
}

// FindByID retrieves a record by its identifier.
func (r *BaseRepository[T]) FindByID(ctx context.Context, id uint64) (T, error) {
	var entity T
	result := r.db.WithContext(ctx).First(&entity, id)
	if result.Error != nil {
		var zero T
		return zero, result.Error
	}
	return entity, nil
}

// FindByField loads the first record matching the provided field condition.
func (r *BaseRepository[T]) FindByField(ctx context.Context, model interface{}, field string, value interface{}) error {
	return r.db.WithContext(ctx).Where(field+" = ?", value).First(model).Error
}

// DeleteByID removes records by primary key.
func (r *BaseRepository[T]) DeleteByID(ctx context.Context, id uint64) error {
	var entity T
	return r.db.WithContext(ctx).Delete(&entity, id).Error
}

// ExistsByID checks if a record exists for the given ID.
func (r *BaseRepository[T]) ExistsByID(ctx context.Context, id uint64) (bool, error) {
	var count int64
	var entity T
	err := r.db.WithContext(ctx).Model(&entity).Where("id = ?", id).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ExistsByField checks uniqueness constraints against a field value.
func (r *BaseRepository[T]) ExistsByField(ctx context.Context, model interface{}, field string, value interface{}) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(model).Where(field+" = ?", value).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// FindWithConditions loads all matching records from the provided condition map.
func (r *BaseRepository[T]) FindWithConditions(ctx context.Context, conditions map[string]interface{}) ([]T, error) {
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

// FindList queries paginated results while filling the consumer-provided model slice.
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
	var entities []T
	if err := db.Find(&entities).Error; err != nil {
		return nil, err
	}
	return entities, nil
}

// CountWithConditions returns the count for the supplied conditions.
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
