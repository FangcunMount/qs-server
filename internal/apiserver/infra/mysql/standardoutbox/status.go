//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	appstandard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

const standardUTCLayout = "2006-01-02 15:04:05.000000"

// StatusReader borrows the host pool. The standard table stores UTC clock
// digits, so timestamps are read as text and parsed as UTC even when the
// host connection uses Asia/Shanghai for business dates.
type StatusReader struct{ db *sql.DB }

func NewStatusReader(db *sql.DB) (*StatusReader, error) {
	if db == nil {
		return nil, fmt.Errorf("standard MySQL status requires host database")
	}
	return &StatusReader{db: db}, nil
}

func (r *StatusReader) OutboxStatusSnapshot(ctx context.Context, now time.Time) (outboxport.StatusSnapshot, error) {
	if r == nil || r.db == nil {
		return outboxport.StatusSnapshot{}, fmt.Errorf("standard MySQL status reader is not configured")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT state, COUNT(*), DATE_FORMAT(MIN(created_at),'%Y-%m-%d %H:%i:%s.%f')
FROM rm_outbox WHERE state <> 'published' GROUP BY state`)
	if err != nil {
		return outboxport.StatusSnapshot{}, err
	}
	defer rows.Close()
	counts := make([]appstandard.StatusCount, 0, 4)
	for rows.Next() {
		var state, oldestText string
		var count int64
		if err := rows.Scan(&state, &count, &oldestText); err != nil {
			return outboxport.StatusSnapshot{}, err
		}
		oldest, err := time.ParseInLocation(standardUTCLayout, oldestText, time.UTC)
		if err != nil {
			return outboxport.StatusSnapshot{}, fmt.Errorf("decode standard MySQL creation time: %w", err)
		}
		counts = append(counts, appstandard.StatusCount{State: state, Count: count, OldestCreatedAt: &oldest})
	}
	if err := rows.Err(); err != nil {
		return outboxport.StatusSnapshot{}, err
	}
	return appstandard.BuildStatusSnapshot("assessment-mysql-outbox", now, counts)
}
