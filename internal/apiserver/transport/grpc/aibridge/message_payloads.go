package aibridge

import (
	"context"
	"errors"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// PayloadReader is supplied by the host. It owns the read transaction and retains
// message bodies until business confirmation plus the reviewed rollback window.
type PayloadReader interface {
	ReadMessagePayload(context.Context, *pb.MessagePayloadReference, string) ([]byte, error)
}

type MessagePayloads struct {
	pb.UnimplementedMessagePayloadsServer
	Reader PayloadReader
}

func (s *MessagePayloads) RegisterService(server *grpc.Server) {
	pb.RegisterMessagePayloadsServer(server, s)
}

func (s *MessagePayloads) Get(ctx context.Context, r *pb.MessagePayloadReference) (*pb.MessagePayload, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "mTLS required")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.PeerCertificates) == 0 || tlsInfo.State.PeerCertificates[0].Subject.CommonName != "qs-ai.svc" {
		return nil, status.Error(codes.PermissionDenied, "untrusted workload")
	}
	if r == nil || r.Producer != "qs-server" || r.Destination != "qs-ai" || len(r.MessageId) != 36 || len(r.BodySha256) != 64 || len(r.OrganizationId) == 0 || len(r.OrganizationId) > 20 || r.BodyLength == 0 || r.BodyLength > app.MaxMessagingBody {
		return nil, status.Error(codes.InvalidArgument, "invalid message reference")
	}
	if s.Reader == nil {
		return nil, status.Error(codes.Unavailable, "payload storage unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	body, err := s.Reader.ReadMessagePayload(ctx, r, "qs-ai")
	if err != nil {
		if errors.Is(err, app.ErrConflict) || errors.Is(err, app.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "message reference unavailable")
		}
		return nil, status.Error(codes.Unavailable, "payload storage unavailable")
	}
	return &pb.MessagePayload{Reference: proto.Clone(r).(*pb.MessagePayloadReference), Body: body}, nil
}
