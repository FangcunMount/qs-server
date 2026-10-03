package aibridge

import (
	"context"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// MessagingPayloadClient borrows the host's preconfigured mTLS connection to
// qs-ai. References contain no endpoint; constructors never connect or close it.
type MessagingPayloadClient struct{ RPC pb.MessagePayloadsClient }

func NewMessagingPayloadClient(conn grpc.ClientConnInterface) *MessagingPayloadClient {
	return &MessagingPayloadClient{RPC: pb.NewMessagePayloadsClient(conn)}
}

// Read only accepts an already authenticated qs-ai event header. The exact body
// bytes and reference are verified before any original business handler runs.
func (c *MessagingPayloadClient) Read(ctx context.Context, envelope *pb.MessagingEnvelope) ([]byte, error) {
	if app.ValidateMessagingHeader(envelope, app.EventsTopic) != nil {
		return nil, app.ErrMessagingContract
	}
	if inline, ok := envelope.Body.(*pb.MessagingEnvelope_InlineBody); ok {
		if _, err := app.ParseMessagingBody(envelope, inline.InlineBody); err != nil {
			return nil, err
		}
		return append([]byte(nil), inline.InlineBody...), nil
	}
	if c == nil || c.RPC == nil {
		return nil, app.ErrManagementUnavailable
	}
	ref := envelope.GetPayloadReference()
	// Freeze the requested reference before crossing a borrowed client boundary.
	ref = proto.Clone(ref).(*pb.MessagePayloadReference)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.Get(ctx, proto.Clone(ref).(*pb.MessagePayloadReference), grpc.MaxCallRecvMsgSize(app.MaxMessagingBody+2048))
	if err != nil {
		return nil, app.ErrManagementUnavailable
	}
	if response == nil || response.Reference == nil || !proto.Equal(response.Reference, ref) {
		return nil, app.ErrMessagingContract
	}
	if _, err := app.ParseMessagingBody(envelope, response.Body); err != nil {
		return nil, err
	}
	return append([]byte(nil), response.Body...), nil
}
