//go:build integration && reliable_messaging

package checkpoint_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// The barrier forces all first reads to observe an absent Run. It leaves the
// real query, original RR isolation and unique constraint intact, and reproduces
// the missing-row gap-lock deadlock of the old Claim implementation.
func TestFirstRunClaimsWithoutMissingRowGapDeadlocks(t *testing.T) {
	dsn := os.Getenv("RM_QS_FIRST_CLAIM_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "mysql:3306", parsed.Addr, "disposable endpoint only")
	require.Equal(t, "m4_qs_first_claim", parsed.DBName)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	db, err := gorm.Open(driver.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	for _, migration := range []struct{ name, marker string }{
		{"000040_merge_runtime_checkpoint.up.sql", "CREATE TABLE IF NOT EXISTS `runtime_checkpoint`"},
		{"000046_add_evaluation_run_claim_lease.up.sql", "ALTER TABLE `runtime_checkpoint`"},
		{"000049_add_retry_governance.up.sql", "ALTER TABLE `runtime_checkpoint`"},
	} {
		source, e := os.ReadFile("../../../../pkg/migration/migrations/mysql/" + migration.name)
		require.NoError(t, e)
		start := strings.Index(string(source), migration.marker)
		require.GreaterOrEqual(t, start, 0)
		statement := string(source)[start:]
		end := strings.Index(statement, ";")
		require.GreaterOrEqual(t, end, 0)
		require.NoError(t, db.Exec(statement[:end+1]).Error)
	}
	var isolation string
	require.NoError(t, db.Raw("SELECT @@transaction_isolation").Scan(&isolation).Error)
	require.Equal(t, "REPEATABLE-READ", isolation)

	for _, sameAssessment := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_assessment_%t", sameAssessment), func(t *testing.T) {
			const concurrency = 16
			gate := make(chan struct{})
			var arrived atomic.Int32
			const callback = "first_claim_absent_read_barrier"
			require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(query *gorm.DB) {
				if query.Statement.Table != "runtime_checkpoint" || !errors.Is(query.Error, gorm.ErrRecordNotFound) {
					return
				}
				n := arrived.Add(1)
				if n == concurrency {
					close(gate)
				}
				if n <= concurrency {
					select {
					case <-gate:
					case <-ctx.Done():
						query.AddError(ctx.Err())
					}
				}
			}))
			defer func() { require.NoError(t, db.Callback().Query().Remove(callback)) }()
			type claim struct {
				result evaluationrun.ClaimResult
				err    error
			}
			claims := make(chan claim, concurrency)
			repo := checkpoint.NewRunRepository(db)
			now := time.Now().Truncate(time.Millisecond)
			for i := range concurrency {
				go func(i int) {
					id := uint64(1000 + i)
					if sameAssessment {
						id = 2000
					}
					result, e := repo.Claim(ctx, evaluationrun.ClaimRequest{AssessmentID: id, Token: fmt.Sprintf("owner-%d", i), ClaimedAt: now, LeaseUntil: now.Add(time.Minute)})
					claims <- claim{result, e}
				}(i)
			}
			var results []evaluationrun.ClaimResult
			for range concurrency {
				claim := <-claims
				require.NoError(t, claim.err, "first claims must not depend on broker requeue")
				results = append(results, claim.result)
			}
			require.EqualValues(t, concurrency, arrived.Load())
			owners := 0
			for _, result := range results {
				require.Equal(t, 1, result.Run.Attempt().Number)
				require.Equal(t, fmt.Sprintf("%d:1", result.Run.AssessmentID()), result.Run.ID().String())
				if result.Claimed {
					owners++
				}
			}
			if sameAssessment {
				require.Equal(t, 1, owners)
				owner, e := repo.FindLatestByAssessmentID(ctx, 2000)
				require.NoError(t, e)
				require.NotNil(t, owner)
				for _, result := range results {
					require.Equal(t, owner.ClaimToken(), result.Run.ClaimToken())
				}
			} else {
				require.Equal(t, concurrency, owners)
			}
		})
	}
	var count int64
	require.NoError(t, db.Table("runtime_checkpoint").Count(&count).Error)
	require.EqualValues(t, 17, count, "sixteen independent Runs and one shared first attempt")
}
