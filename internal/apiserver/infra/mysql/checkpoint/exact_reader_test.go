package checkpoint

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/stretchr/testify/require"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestExactRunReaderDoesNotChooseLatestOrAmbiguousAttempt(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		t.Run(string(rune('0'+count)), func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			require.NoError(t, err)
			db, err := gorm.Open(driver.New(driver.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
			require.NoError(t, err)
			t.Cleanup(func() {
				mock.ExpectClose()
				require.NoError(t, pool.Close())
				require.NoError(t, mock.ExpectationsWereMet())
			})
			id := evalrun.NewEvaluationRunWithAttempt(42, 1).ID()
			rows := sqlmock.NewRows([]string{"scope", "resource_id", "assessment_id", "attempt_no", "status"})
			for range count {
				rows.AddRow(scopeEvaluationRun, id.String(), 42, 1, "failed")
			}
			mock.ExpectQuery("SELECT (.+) FROM `runtime_checkpoint`").WithArgs(scopeEvaluationRun, id.String(), 2).WillReturnRows(rows)
			got, err := NewExactRunReader(db).FindRunByID(t.Context(), id)
			if count == 2 {
				require.Error(t, err)
				require.Nil(t, got)
			} else {
				require.NoError(t, err)
				if count == 0 {
					require.Nil(t, got)
				} else {
					require.Equal(t, id, got.ID())
					require.Equal(t, 1, got.Attempt().Number)
				}
			}
		})
	}
}
