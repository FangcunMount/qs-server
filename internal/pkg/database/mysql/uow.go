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
func (u *UnitOfWork) WithinTransaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	if fn == nil {
		return nil
	}
	if u == nil || u.db == nil {
		return fn(nil)
	}

	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	})
}
