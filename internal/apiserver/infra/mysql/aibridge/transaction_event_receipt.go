package aibridge

import (
	"context"
	"database/sql"
	"strconv"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

// The receipt is a business fact in the caller's original root. A new command
// has no delivery Outbox to confirm; its immutable exact witness proves which
// original command was committed. Neither path commits this root or sends ACK.
func (s *MessagingStore) transactionCommandReceipt(ctx context.Context, tx *sql.Tx, id, organization, resource, aggregate string, kind pb.MessagingKind, hash string) (bool, error) {
	a, found, err := s.Archive.readCommittedInRoot(ctx, tx, id)
	if err != nil {
		return true, err
	}
	if !found {
		_, planned, err := s.Archive.commandRequestIn(ctx, tx, organization, id)
		if err != nil {
			return true, err
		}
		if planned {
			return true, app.ErrMessagingContract // a Prepared/new identity cannot become legacy settlement
		}
		return false, nil
	}
	body, err := app.ParseMessagingBody(a.Envelope, a.Body)
	if err != nil {
		return true, err
	}
	scope, err := app.CommandScope(a.Envelope, body)
	if err != nil || scope.OrganizationID != organization || scope.ResourceID != resource || strconv.FormatUint(a.OrganizationID, 10) != organization || a.Envelope.MessageId != id || a.Envelope.AggregateKey != aggregate || a.Envelope.Kind != kind || a.Envelope.BodySha256 != hash || !a.Ordered || !a.RequiresReceipt {
		return true, app.ErrMessagingContract
	}
	return true, nil
}
