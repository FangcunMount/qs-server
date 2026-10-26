package aibridge

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestRuntimeSummaryUsesCurrentCommittedCommandEvidence(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(7), int64(7), "request", int64(7), int64(7), "request").WillReturnRows(sqlmock.NewRows([]string{"invalid"}).AddRow(false))
	mock.ExpectQuery("(?s)SELECT .*ai_messaging_archive.*rm_tx_execution.*rm_tx_witness.*").WithArgs(int64(7), "request").WillReturnRows(sqlmock.NewRows([]string{"request", "session", "testee", "status", "version", "created", "updated", "assessments", "pending", "attempts"}).AddRow("request", "session", "19", "running", 1, nil, nil, []byte(`["21"]`), 1, 0))
	result, err := (&Store{DB: db}).GetRuntime(t.Context(), 7, "request")
	require.NoError(t, err)
	require.Equal(t, 1, result.CommandsPending)
	require.NoError(t, mock.ExpectationsWereMet())
}
