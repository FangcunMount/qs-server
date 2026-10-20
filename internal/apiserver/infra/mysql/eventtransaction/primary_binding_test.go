package eventtransaction

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"testing"
)

func TestSharedOwnerRequiresSameOriginalPrimary(t *testing.T) {
	makeDB := func() (*gorm.DB, sqlmock.Sqlmock) {
		pool, mock, err := sqlmock.New()
		require.NoError(t, err)
		db, err := gorm.Open(driver.New(driver.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
		require.NoError(t, err)
		t.Cleanup(func() {
			mock.ExpectClose()
			require.NoError(t, pool.Close())
			require.NoError(t, mock.ExpectationsWereMet())
		})
		return db, mock
	}
	db, mock := makeDB()
	other, _ := makeDB()
	pool, err := db.DB()
	require.NoError(t, err)
	c := &Committer{primary: pool}
	require.True(t, c.UsesPrimary(db))
	require.False(t, c.UsesPrimary(other))
	require.False(t, c.UsesPrimary(nil))
	mock.ExpectBegin()
	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.False(t, c.UsesPrimary(tx), "root transaction cannot become owner primary")
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback().Error)
}
