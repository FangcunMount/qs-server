// Package outboxhistory resolves the source for read-only legacy evidence.
// The active Outbox and its write paths keep their own physical table.
package outboxhistory

import (
	"context"
	"database/sql"
	"errors"

	"gorm.io/gorm"
)

const (
	LegacyTable = "rm_outbox"
	View        = "qs_rm_outbox_history"
)

// lookupView uses only metadata available to readers with SELECT on the
// compatibility view. A present view whose dependencies are unavailable must
// fail on the caller's query, without falling back to a different dataset.
const lookupView = `SELECT TABLE_TYPE FROM information_schema.TABLES
	WHERE TABLE_SCHEMA=DATABASE() AND BINARY TABLE_NAME='qs_rm_outbox_history'`

type rowQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func Source(ctx context.Context, db rowQuery) (string, error) {
	var kind string
	err := db.QueryRowContext(ctx, lookupView).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return LegacyTable, nil
	}
	if err != nil {
		return "", err
	}
	if kind != "VIEW" {
		return "", errors.New("MySQL Outbox history source is not a compatibility view")
	}
	return View, nil
}

// GORM keeps metadata lookup on the same connection or transaction as the
// data read. Existing non-MySQL test adapters use the legacy table.
func GORM(ctx context.Context, db *gorm.DB) (string, error) {
	if db.Dialector.Name() != "mysql" {
		return LegacyTable, nil
	}
	return Source(ctx, db.Statement.ConnPool)
}
