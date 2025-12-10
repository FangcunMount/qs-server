package mysql

import (
	"context"

	"gorm.io/gorm"
)

// UnitOfWork wraps a GORM DB to offer transactional execution helpers.
type UnitOfWork struct {
	db *gorm.DB
}

// NewUnitOfWork constructs a UnitOfWork for the given *gorm.DB.
func NewUnitOfWork(db *gorm.DB) *UnitOfWork {
	if db == nil {
		return nil
	}
	return &UnitOfWork{db: db}
}

// WithinTransaction executes fn inside a database transaction.
// The transaction is automatically injected into the context so that
// Repository operations can use it transparently.
//
// 使用方式：
//
//	err := uow.WithinTransaction(ctx, func(txCtx context.Context) error {
//	    // 在这里调用 Repository 方法，它们会自动使用事务
//	    return repo.Save(txCtx, entity)
//	})
func (u *UnitOfWork) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	if fn == nil {
		return nil
	}
	if u == nil || u.db == nil {
		return fn(ctx)
	}

	// 检查数据库连接状态
	sqlDB, err := u.db.DB()
	if err != nil {
		return err
	}
	if err := sqlDB.Ping(); err != nil {
		return err
	}

	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 将事务注入到 context 中，使用类型安全的 key
		txCtx := context.WithValue(ctx, txKey{}, tx)
		return fn(txCtx)
	})
}
