package aibridge

import (
	"context"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	"github.com/FangcunMount/qs-server/internal/pkg/aidiagnostics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type Receiver struct {
	pb.UnimplementedResultsServer
}

func (r *Receiver) RegisterService(server *grpc.Server) {
	pb.RegisterResultsServer(server, r)
}

func (r *Receiver) Accept(ctx context.Context, e *pb.StateEvent) (ack *pb.Acknowledgement, resultErr error) {
	correlation := aidiagnostics.IncomingCorrelation(ctx)
	p, ok := peer.FromContext(ctx)
	if !ok {
		aidiagnostics.Boundary("delivery_retry", "rejected", "permission_denied", correlation, e.GetRequestId(), e.GetEventId())
		return nil, status.Error(codes.PermissionDenied, "mTLS required")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.PeerCertificates) == 0 || tlsInfo.State.PeerCertificates[0].Subject.CommonName != "qs-ai.svc" {
		aidiagnostics.Boundary("delivery_retry", "rejected", "permission_denied", correlation, e.GetRequestId(), e.GetEventId())
		return nil, status.Error(codes.PermissionDenied, "untrusted workload")
	}
	aidiagnostics.Boundary("delivery_rejected", "rejected", "transport_retired", correlation, e.GetRequestId(), e.GetEventId())
	return nil, status.Error(codes.FailedPrecondition, "result delivery requires MQ")
}
