package aibridge

import (
	"database/sql"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMessagingWritesRequireTransactionOwnerBeforeStorage(t *testing.T) {
	pool := new(sql.DB) // inert and unusable: reaching storage would panic
	commands := &MessagingCommandStore{Store: &Store{DB: pool}, Messaging: NewMessagingStore(), Seal: func(pb.MessagingKind, string, string, string, string, *pb.MessagingBody) (*app.PreparedMessaging, error) {
		t.Fatal("retired path reached Seal")
		return nil, nil
	}}
	require.ErrorIs(t, commands.StageStart(t.Context(), app.Start{}), app.ErrManagementUnavailable)
	require.ErrorIs(t, commands.StageChange(t.Context(), "", app.Change{}), app.ErrManagementUnavailable)
	require.ErrorIs(t, commands.SubmitParticipantRetry(t.Context(), app.DraftScope{}, "", app.ParticipantRetry{}), app.ErrManagementUnavailable)
	require.ErrorIs(t, commands.SubmitEvaluationStart(t.Context(), app.EvaluationScope{}, "", app.EvaluationStart{}), app.ErrManagementUnavailable)
	require.ErrorIs(t, commands.SubmitEvaluationCancel(t.Context(), app.EvaluationScope{}, "", app.EvaluationCancel{}), app.ErrManagementUnavailable)
	receiver := &MessagingEventReceiver{DB: pool, Store: NewMessagingStore()}
	require.ErrorIs(t, receiver.Receive(t.Context(), []byte("untrusted")), app.ErrManagementUnavailable)
	require.ErrorIs(t, receiver.Failed(t.Context(), []byte("untrusted")), app.ErrManagementUnavailable)
	require.ErrorIs(t, receiver.FailedHandoff(t.Context(), nil, nil), app.ErrManagementUnavailable)
}
