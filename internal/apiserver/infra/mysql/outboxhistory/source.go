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

// ArchiveSource never substitutes the live ledger for an explicit archive audit.
func ArchiveSource(ctx context.Context, db rowQuery) (string, error) {
	source, err := Source(ctx, db)
	if err != nil {
		return "", err
	}
	if source != View {
		return "", errors.New("MySQL Outbox history view is unavailable")
	}
	return source, nil
}

// ActiveSource requires the actual physical table for live inventory.
func ActiveSource(ctx context.Context, db rowQuery) (string, error) {
	var kind string
	err := db.QueryRowContext(ctx, `SELECT TABLE_TYPE FROM information_schema.TABLES
		WHERE TABLE_SCHEMA=DATABASE() AND BINARY TABLE_NAME='rm_outbox'`).Scan(&kind)
	if err != nil {
		return "", err
	}
	if kind != "BASE TABLE" {
		return "", errors.New("active MySQL Outbox source is not a physical table")
	}
	return LegacyTable, nil
}

var ErrUnresolvedArchive = errors.New("MySQL Outbox history still contains unresolved publication responsibilities")

// OperationalSource keeps backlog and recovery status on the live table while
// it exists. An archive cannot turn unfinished publication into a live candidate
// or an empty healthy snapshot. Exact held-to-schedule matches are verified by
// the separate responsibility inventory.
func OperationalSource(ctx context.Context, db rowQuery) (string, error) {
	source, err := ActiveSource(ctx, db)
	if err == nil {
		return source, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	source, err = ArchiveSource(ctx, db)
	if err != nil {
		return "", err
	}
	var unresolved bool
	err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM `+source+` AS o
		WHERE o.state NOT IN ('published','held') OR o.claim_token IS NOT NULL OR o.lease_until IS NOT NULL
		OR (o.state='published' AND (o.transport_confirmed_at IS NULL OR o.transport_confirmed_at<'1970-01-01'))
		OR (o.state='held' AND (o.transport_confirmed_at IS NOT NULL OR o.attempt_count<>0 OR o.failure_count<>0
			OR o.manual_replay_request_id IS NOT NULL OR o.manual_replay_version IS NOT NULL OR o.version=0
			OR NOT EXISTS (SELECT 1 FROM qs_retry_schedule AS s
				WHERE s.owner='qs.mysql' AND s.scope=o.scope AND s.message_id=o.message_id
				AND BINARY o.last_error_code=CONCAT('m3_future_owner:', LOWER(HEX(s.plan_hash)))))))`).Scan(&unresolved)
	if err != nil {
		return "", err
	}
	if unresolved {
		return "", ErrUnresolvedArchive
	}
	return source, nil
}

// GORM keeps metadata lookup on the same connection or transaction as the
// data read. Existing non-MySQL test adapters use the legacy table.
func GORM(ctx context.Context, db *gorm.DB) (string, error) {
	if db.Name() != "mysql" {
		return LegacyTable, nil
	}
	return Source(ctx, db.Statement.ConnPool)
}
