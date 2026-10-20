package aibridge

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

func TestPreparedACKCannotFallBackToHistoricalPayloadWithSameIdentity(t *testing.T) {
	a, mock, _ := commandArchiveFixture(t)
	_, execution, ack := eventExecutionFixture(t, false)
	mock.ExpectQuery("SELECT organization_id,frozen_payload,first_wire,execution_key,branch_key,reference FROM ai_messaging_archive").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT owner,scope,idempotency_key,input_hash FROM rm_tx_request").WillReturnError(sql.ErrNoRows)
	bk := transaction.BranchKey(ack.Envelope.MessageId)
	raw, err := transaction.EncodeReference(execution.References()[0])
	require.NoError(t, err)
	mock.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME\\(1000\\) \\*/ w.reference FROM rm_tx_witness").WithArgs([]byte("qs.mysql"), bk[:]).WillReturnRows(sqlmock.NewRows([]string{"ref"}).AddRow(raw))
	_, found, err := a.Payload(t.Context(), &pb.MessagePayloadReference{Producer: "qs-server", Destination: "qs-ai", MessageId: ack.Envelope.MessageId, OrganizationId: "7", BodySha256: ack.Envelope.BodySha256, BodyLength: uint64(len(ack.Body))}, "qs-ai")
	require.True(t, found)
	require.ErrorIs(t, err, app.ErrNotFound)
}
