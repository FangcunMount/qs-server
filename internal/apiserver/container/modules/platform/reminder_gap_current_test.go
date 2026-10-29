package platform

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestCurrentHostCannotEnableHistoricalReminderRequeue(t *testing.T) {
	conn, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, conn.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	facade := BuildRESTSystemGovernanceFacade(RESTSystemGovernanceInput{
		MySQLDB: db, TaskOpenedReminderEnabled: true,
		Options: &options.SystemGovernanceOptions{Retry: &options.RetryGovernanceOptions{GapRecoveryEnabled: true}},
		// Current RocketMQ composition has no publishing Outbox readers.
	})
	req := app.ReminderGapRecoveryRequest{RequestID: "current-reminder-gap", TaskID: "42", OpeningEventID: "original-opening", ExpectedVersion: 1, OpenedBefore: time.Now().Add(-time.Minute), EvidenceReference: "fixture", Reason: "reviewed", Confirm: true}
	require.NoError(t, app.ValidateReminderGapRecovery(7, 11, req))
	recovery, ok := facade.(app.ReminderGapRecovery)
	require.True(t, ok)
	_, err = recovery.AuthorizeReminderGap(t.Context(), 7, 11, req)
	require.Error(t, err)
	require.True(t, baseerrors.IsCode(err, code.ErrInternalServerError))
	// No query/mutation may reach the historical requeue writer.
	require.NoError(t, mock.ExpectationsWereMet())
}
