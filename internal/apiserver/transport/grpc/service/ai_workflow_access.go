package service

import (
	"context"
	"errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type AIWorkflowAccessService struct {
	pb.UnimplementedAIWorkflowAccessServiceServer
	Access interface {
		Authorize(context.Context, app.Actor, string, []string) error
	}
}

func (s *AIWorkflowAccessService) RegisterService(server *grpc.Server) {
	pb.RegisterAIWorkflowAccessServiceServer(server, s)
}
func (s *AIWorkflowAccessService) Authorize(ctx context.Context, r *pb.AIWorkflowAccessRequest) (*pb.AIWorkflowAccessResponse, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "mTLS required")
	}
	tls, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tls.State.VerifiedChains) == 0 || len(tls.State.PeerCertificates) == 0 || tls.State.PeerCertificates[0].Subject.CommonName != "qs-ai.svc" {
		return nil, status.Error(codes.PermissionDenied, "untrusted workload")
	}
	if s.Access == nil {
		return nil, status.Error(codes.Unavailable, "authorization unavailable")
	}
	err := s.Access.Authorize(ctx, app.Actor{OrgID: r.GetOrgId(), SubjectID: r.GetSubjectId()}, r.GetTesteeId(), r.GetAssessmentIds())
	if err != nil {
		switch {
		case errors.Is(err, app.ErrInvalid):
			return nil, status.Error(codes.InvalidArgument, "invalid access request")
		case errors.Is(err, app.ErrAccessDenied):
			return nil, status.Error(codes.PermissionDenied, "access denied")
		case errors.Is(err, app.ErrAccessUnavailable):
			return nil, status.Error(codes.Unavailable, "authorization unavailable")
		}
		code := status.Code(toAssessmentQueryGRPCError(err))
		if code == codes.Internal {
			code = codes.Unavailable
		}
		return nil, status.Error(code, "authorization not granted")
	}
	return &pb.AIWorkflowAccessResponse{}, nil
}
