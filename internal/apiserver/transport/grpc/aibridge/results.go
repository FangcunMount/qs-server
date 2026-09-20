package aibridge

import (
	"context"
	"errors"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	infra "github.com/FangcunMount/qs-server/internal/apiserver/infra/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type Receiver struct {
	pb.UnimplementedResultsServer
	Service *app.Service
}

func (r *Receiver) RegisterService(server *grpc.Server) {
	pb.RegisterResultsServer(server, r)
}

func (r *Receiver) Accept(ctx context.Context, e *pb.StateEvent) (ack *pb.Acknowledgement, resultErr error) {
	correlation := infra.CorrelationFromIncoming(ctx)
	p, ok := peer.FromContext(ctx)
	if !ok {
		infra.EmitBoundary("delivery_retry", "rejected", "permission_denied", correlation, e.GetRequestId(), e.GetEventId())
		return nil, status.Error(codes.PermissionDenied, "mTLS required")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.PeerCertificates) == 0 || tlsInfo.State.PeerCertificates[0].Subject.CommonName != "qs-ai.svc" {
		infra.EmitBoundary("delivery_retry", "rejected", "permission_denied", correlation, e.GetRequestId(), e.GetEventId())
		return nil, status.Error(codes.PermissionDenied, "untrusted workload")
	}
	if r.Service == nil {
		infra.EmitBoundary("delivery_retry", "rejected", "dependency_unavailable", correlation, e.GetRequestId(), e.GetEventId())
		return nil, status.Error(codes.Unavailable, "receiver unavailable")
	}
	event := app.Event{EventID: e.EventId, RequestID: e.RequestId, SessionID: e.SessionId, Actor: app.Actor{OrgID: e.GetActor().GetOrgId(), SubjectID: e.GetActor().GetSubjectId()}, TesteeID: e.TesteeId, Version: e.Version, Status: e.Status, QuestionID: e.QuestionId, Question: e.Question, CanSkip: e.CanSkip, FailureCode: e.FailureCode}
	event.ArtifactJSON = e.ArtifactJson
	if err := r.Service.Accept(ctx, event); err != nil {
		code := codes.Unavailable
		errorClass := "delivery_failed"
		switch {
		case errors.Is(err, app.ErrInvalid):
			code = codes.InvalidArgument
			errorClass = "invalid_argument"
		case errors.Is(err, app.ErrConflict):
			code = codes.Aborted
			// Duplicate confirmation is an idempotent hit for ops triage.
			infra.EmitBoundary("delivery_confirmed", "duplicate", "", correlation, e.GetRequestId(), e.GetEventId())
			return nil, status.Error(code, "result not accepted")
		case errors.Is(err, app.ErrNotFound):
			code = codes.NotFound
			errorClass = "not_found"
		}
		infra.EmitBoundary("delivery_retry", "rejected", errorClass, correlation, e.GetRequestId(), e.GetEventId())
		return nil, status.Error(code, "result not accepted")
	}
	infra.EmitBoundary("delivery_confirmed", "ok", "", correlation, e.GetRequestId(), e.GetEventId())
	return &pb.Acknowledgement{EventId: e.EventId}, nil
}
