package migration

import (
	"context"
	"database/sql/driver"
	"errors"
	"time"

	sqlmysql "github.com/go-sql-driver/mysql"
)

const emptyAIPublisherSQLVersion = 111

// The surrounding driver has already verified the exact embedded body, dirty
// version, selected connection and paired database identity. Count and DROP must
// share one session's table lock; a prior read-only receipt is insufficient.
func (d *retirementDatabaseDriver) runEmptyAIPublisherStep(raw []byte) (runErr error) {
	if d.backend != BackendMySQL || d.sqlConn == nil || d.version != emptyAIPublisherSQLVersion {
		return retirementError("empty publisher retirement requires the bound mysql step")
	}
	ctx, cancel := context.WithTimeout(d.ctx, 8*time.Second)
	defer cancel()
	var originalWait uint64
	if err := d.sqlConn.QueryRowContext(ctx, "SELECT @@SESSION.lock_wait_timeout").Scan(&originalWait); err != nil {
		return retirementError("publisher lock wait configuration unavailable")
	}
	locked := false
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(d.ctx), 5*time.Second)
		defer cleanupCancel()
		var cleanupErr error
		if locked {
			_, err := d.sqlConn.ExecContext(cleanupCtx, "UNLOCK TABLES")
			cleanupErr = errors.Join(cleanupErr, err)
		}
		_, err := d.sqlConn.ExecContext(cleanupCtx, "SET @qs_m5_empty_ai_publisher_guard = NULL, @qs_m5_ai_publisher_sql = NULL")
		cleanupErr = errors.Join(cleanupErr, err)
		_, err = d.sqlConn.ExecContext(cleanupCtx, "DEALLOCATE PREPARE qs_m5_ai_publisher_stmt")
		var mysqlErr *sqlmysql.MySQLError
		// Preparation may not have happened (e.g. a nonempty table). Only the
		// server's exact absent-statement code is an expected cleanup result.
		if err != nil && !(errors.As(err, &mysqlErr) && mysqlErr.Number == 1243) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
		_, err = d.sqlConn.ExecContext(cleanupCtx, "SET SESSION lock_wait_timeout = ?", originalWait)
		cleanupErr = errors.Join(cleanupErr, err)
		if cleanupErr != nil {
			// A session with uncertain locks/variables must never return to the
			// application's borrowed pool. ErrBadConn closes its physical socket.
			_ = d.sqlConn.Raw(func(any) error { return driver.ErrBadConn })
			runErr = errors.Join(runErr, retirementError("publisher session cleanup failed; connection discarded"))
		}
	}()
	if _, err := d.sqlConn.ExecContext(ctx, "SET SESSION lock_wait_timeout = 2"); err != nil {
		return retirementError("publisher bounded lock wait unavailable")
	}
	if _, err := d.sqlConn.ExecContext(ctx, "LOCK TABLES `ai_messaging_outbox` WRITE"); err != nil {
		return retirementError("publisher exclusive table lock unavailable")
	}
	locked = true
	var count uint64
	if err := d.sqlConn.QueryRowContext(ctx, "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM `ai_messaging_outbox`").Scan(&count); err != nil {
		return retirementError("publisher empty-state proof unavailable")
	}
	if count != 0 {
		return retirementError("publisher records remain; preserve every original record")
	}
	if _, err := d.sqlConn.ExecContext(ctx, "SET @qs_m5_empty_ai_publisher_guard = 1"); err != nil {
		return retirementError("publisher execution binding unavailable")
	}
	if _, err := d.sqlConn.ExecContext(ctx, string(raw)); err != nil {
		return retirementError("bound empty publisher retirement failed")
	}
	return nil
}
