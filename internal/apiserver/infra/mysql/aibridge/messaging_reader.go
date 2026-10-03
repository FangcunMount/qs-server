package aibridge

import (
	"context"
	"database/sql"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
)

// MessagingReader is a host read boundary over a borrowed database pool.
// No resource is created or closed; the read transaction never writes or commits.
type MessagingReader struct {
	DB    *sql.DB
	Store *MessagingStore
}

func (s *MessagingReader) ReadMessagePayload(ctx context.Context, ref *pb.MessagePayloadReference, workload string) ([]byte, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	return s.Store.Payload(ctx, tx, ref, workload)
}

// ObserveMessagePayloadFailure does not reuse or commit the body-read transaction.
func (s *MessagingReader) ObserveMessagePayloadFailure(ctx context.Context, kind string) error {
	return RecordPayloadObservation(ctx, s.DB, kind)
}
