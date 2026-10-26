package aibridge

import (
	"context"
	"database/sql"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

// MessagingReader is a host read boundary over a borrowed database pool.
// No resource is created or closed; the read transaction never writes or commits.
type MessagingReader struct {
	DB      *sql.DB
	Archive *TransactionArchive
}

func (s *MessagingReader) ReadMessagePayload(ctx context.Context, ref *pb.MessagePayloadReference, workload string) ([]byte, error) {
	if s == nil || s.Archive == nil {
		return nil, app.ErrManagementUnavailable
	}
	body, found, err := s.Archive.Payload(ctx, ref, workload)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, app.ErrNotFound
	}
	return body, nil

}

// ObserveMessagePayloadFailure does not reuse or commit the body-read transaction.
func (s *MessagingReader) ObserveMessagePayloadFailure(ctx context.Context, kind string) error {
	return RecordPayloadObservation(ctx, s.DB, kind)
}
