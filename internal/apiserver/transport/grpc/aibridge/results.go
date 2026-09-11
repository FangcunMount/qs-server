package aibridge

import (
	"context"
	"errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type Receiver struct {
	pb.UnimplementedResultsServer
	Service *app.Service
}

func (r *Receiver) Accept(ctx context.Context, e *pb.StateEvent) (*pb.Acknowledgement, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "mTLS required")
	}
	tls, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tls.State.VerifiedChains) == 0 || len(tls.State.PeerCertificates) == 0 || tls.State.PeerCertificates[0].Subject.CommonName != "qs-ai.svc" {
		return nil, status.Error(codes.PermissionDenied, "untrusted workload")
	}
	if r.Service == nil {
		return nil, status.Error(codes.Unavailable, "receiver unavailable")
	}
	event := app.Event{EventID: e.EventId, RequestID: e.RequestId, SessionID: e.SessionId, Actor: app.Actor{OrgID: e.GetActor().GetOrgId(), SubjectID: e.GetActor().GetSubjectId()}, TesteeID: e.TesteeId, Version: e.Version, Status: e.Status, QuestionID: e.QuestionId, Question: e.Question, CanSkip: e.CanSkip, FailureCode: e.FailureCode}
	event.ArtifactJSON = e.ArtifactJson
	if err := r.Service.Accept(ctx, event); err != nil {
		code := codes.Unavailable
		switch {
		case errors.Is(err, app.ErrInvalid):
			code = codes.InvalidArgument
		case errors.Is(err, app.ErrConflict):
			code = codes.Aborted
		case errors.Is(err, app.ErrNotFound):
			code = codes.NotFound
		}
		return nil, status.Error(code, "result not accepted")
	}
	return &pb.Acknowledgement{EventId: e.EventId}, nil
}
