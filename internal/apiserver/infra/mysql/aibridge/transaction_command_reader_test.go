package aibridge

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func expectReadCommandArchive(t *testing.T, mock sqlmock.Sqlmock, e transaction.Execution, state transaction.State) app.FrozenMessagingArchive {
	t.Helper()
	c, a, err := app.ReadTransactionCommand(e.Plan)
	require.NoError(t, err)
	ek := transaction.ExecutionKey("qs.mysql", c.ID(), e.WriterEpoch, e.Fence)
	bk := transaction.BranchKey(c.ID())
	ref, err := transaction.EncodeReference(e.References()[0])
	require.NoError(t, err)
	branch := e.Plan.Input().Branches[0]
	mock.ExpectQuery("SELECT organization_id,frozen_payload,first_wire,execution_key,branch_key,reference FROM ai_messaging_archive").WithArgs([]byte("qs-server"), []byte("qs-ai"), []byte(c.ID())).WillReturnRows(sqlmock.NewRows([]string{"org", "payload", "wire", "execution", "branch", "reference"}).AddRow(7, branch.Message.Input().Payload, branch.Wire, ek[:], bk[:], ref))
	mock.ExpectQuery("SELECT w.reference,e.state,w.state FROM rm_tx_execution").WithArgs(ek[:], bk[:], []byte("test-primary"), 1).WillReturnRows(sqlmock.NewRows([]string{"reference", "execution", "branch"}).AddRow(ref, string(state), string(state)))
	return a
}

func TestTransactionCommandOperationReadsNewArchiveWithoutOutbox(t *testing.T) {
	for _, name := range []string{"submitted", "accepted", "held", "wrong-actor", "wrong-org", "wrong-aggregate", "wrong-row", "wrong-receipt"} {
		t.Run(name, func(t *testing.T) {
			a, mock, e := commandArchiveFixture(t)
			archive := expectReadCommandArchive(t, mock, e, transaction.Committed)
			s := &TransactionCommands{archive: a}
			scope := app.OperationScope{OrganizationID: "7", SubjectID: "operator:9"}
			requestID := orderAggregate
			switch name {
			case "wrong-actor":
				scope.SubjectID = "other"
			case "wrong-org":
				scope.OrganizationID = "8"
			case "wrong-aggregate":
				requestID = orderCommand
			default:
				decision, code := "", ""
				var raw []byte
				var receiptID, decided any
				hash := archive.Envelope.BodySha256
				if name == "wrong-row" {
					hash = strings.Repeat("f", 64)
				}
				if name == "accepted" || name == "held" || name == "wrong-receipt" {
					decision = "accepted"
					r := &pb.MessagingCommandReceipt{CommandId: orderCommand, CommandBodySha256: hash, Decision: pb.MessagingDecision_ACCEPTED, OriginalReceipt: &pb.MessagingCommandReceipt_WorkflowReceipt{WorkflowReceipt: &pb.Receipt{SessionId: orderSession, Version: 2, Status: "running"}}}
					if name == "held" {
						decision, r.Decision = "held", pb.MessagingDecision_HELD
					}
					if name == "wrong-receipt" {
						r.CommandId = orderAggregate
					}
					var err error
					raw, err = proto.Marshal(&pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: r}})
					require.NoError(t, err)
					receiptID, decided = "e1941896-e7df-4b9d-9417-b80bd05276b9", time.Now()
				}
				mock.ExpectQuery("SELECT kind,body_sha256,resource_id,aggregate_key,aggregate_sequence,decision,code,receipt_id,receipt,decided_at FROM ai_messaging_operations").WithArgs(orderCommand, "7", "operator:9").WillReturnRows(sqlmock.NewRows([]string{"kind", "hash", "resource", "aggregate", "sequence", "decision", "code", "receipt_id", "receipt", "decided_at"}).AddRow(archive.Envelope.Kind, hash, orderSession, orderAggregate, 1, decision, code, receiptID, raw, decided))
			}
			result, found, err := s.operation(t.Context(), scope, requestID, orderCommand)
			require.True(t, found)
			switch name {
			case "wrong-actor", "wrong-org", "wrong-aggregate":
				require.ErrorIs(t, err, app.ErrNotFound)
			case "wrong-row", "wrong-receipt":
				require.ErrorIs(t, err, app.ErrMessagingContract)
			default:
				require.NoError(t, err)
				require.Equal(t, name, result.Status)
				if name == "submitted" {
					require.Equal(t, "awaiting_receipt", result.TransportStatus)
					require.Empty(t, result.Receipt)
				} else {
					require.NotEmpty(t, result.Receipt)
				}
			}
		})
	}
}

func TestTransactionCommandReaderNeverFallsBackForPreparedArchive(t *testing.T) {
	a, mock, e := commandArchiveFixture(t)
	expectReadCommandArchive(t, mock, e, transaction.Prepared)
	s := &MessagingCommandStore{Store: &Store{DB: a.db}, Messaging: NewMessagingStore(), Transactions: &TransactionCommands{archive: a}}
	_, err := s.ReadOperation(t.Context(), app.OperationScope{OrganizationID: "7", SubjectID: "operator:9"}, orderCommand)
	require.ErrorIs(t, err, app.ErrMessagingContract)
	// There is no expectation for a legacy Outbox lookup.
}

func TestTransactionPayloadRequiresExactCommittedWitnessTenantRecipientAndHash(t *testing.T) {
	for _, name := range []string{"committed", "prepared", "wrong-org", "wrong-hash", "wrong-workload"} {
		t.Run(name, func(t *testing.T) {
			a, mock, e := commandArchiveFixture(t)
			_, archive, err := app.ReadTransactionCommand(e.Plan)
			require.NoError(t, err)
			ref := &pb.MessagePayloadReference{Producer: "qs-server", Destination: "qs-ai", MessageId: orderCommand, OrganizationId: "7", BodySha256: archive.Envelope.BodySha256, BodyLength: uint64(len(archive.Body))}
			workload := "qs-ai"
			if name == "wrong-workload" {
				workload = "other"
			} else {
				state := transaction.Committed
				if name == "prepared" {
					state = transaction.Prepared
				}
				expectReadCommandArchive(t, mock, e, state)
			}
			if name == "wrong-org" {
				ref.OrganizationId = "8"
			}
			if name == "wrong-hash" {
				ref.BodySha256 = strings.Repeat("f", 64)
			}
			body, found, err := a.Payload(t.Context(), ref, workload)
			require.True(t, found)
			if name == "committed" {
				require.NoError(t, err)
				require.Equal(t, archive.Body, body)
			} else {
				require.Error(t, err)
				require.Empty(t, body)
			}
		})
	}
}

func TestTransactionOperationAbsenceAloneAllowsLegacyFallback(t *testing.T) {
	a, mock, _ := commandArchiveFixture(t)
	mock.ExpectQuery("SELECT organization_id,frozen_payload,first_wire,execution_key,branch_key,reference FROM ai_messaging_archive").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT owner,scope,idempotency_key,input_hash FROM rm_tx_request").WillReturnError(sql.ErrNoRows)
	_, found, err := (&TransactionCommands{archive: a}).operation(t.Context(), app.OperationScope{OrganizationID: "7", SubjectID: "operator:9"}, "", orderCommand)
	require.NoError(t, err)
	require.False(t, found)
}
