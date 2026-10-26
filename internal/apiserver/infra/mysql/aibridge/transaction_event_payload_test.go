package aibridge

import (
	"database/sql"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/stretchr/testify/require"
)

func TestPreparedACKCannotFallBackToHistoricalPayloadWithSameIdentity(t *testing.T) {
	a, mock, _ := commandArchiveFixture(t)
	_, _, ack := eventExecutionFixture(t, false)
	mock.ExpectQuery("SELECT organization_id,frozen_payload,first_wire,execution_key,branch_key,reference FROM ai_messaging_archive").WillReturnError(sql.ErrNoRows)
	_, found, err := a.Payload(t.Context(), &pb.MessagePayloadReference{Producer: "qs-server", Destination: "qs-ai", MessageId: ack.Envelope.MessageId, OrganizationId: "7", BodySha256: ack.Envelope.BodySha256, BodyLength: uint64(len(ack.Body))}, "qs-ai")
	require.True(t, found)
	require.ErrorIs(t, err, app.ErrNotFound)
}
