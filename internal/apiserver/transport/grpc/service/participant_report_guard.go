package service

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	pkggrpc "github.com/FangcunMount/qs-server/internal/pkg/grpc"
)

func (s *ParticipantReportService) authorizeDelegatedSubject(ctx context.Context, testeeID uint64, purpose string) error {
	if s == nil || s.delegatedVerifier == nil || !s.delegatedVerifier.Enabled() {
		return nil
	}
	if identity, ok := pkggrpc.ServiceIdentityFromMTLSContext(ctx); ok {
		if err := s.delegatedVerifier.AllowWorkload(identity.ServiceID); err != nil {
			return status.Error(codes.PermissionDenied, err.Error())
		}
	}
	token, err := delegatedsubject.FromIncomingContext(ctx, s.delegatedVerifier, purpose, testeeID)
	if err != nil {
		switch {
		case errors.Is(err, delegatedsubject.ErrMissingToken),
			errors.Is(err, delegatedsubject.ErrInvalidToken),
			errors.Is(err, delegatedsubject.ErrExpiredToken):
			return status.Error(codes.Unauthenticated, err.Error())
		default:
			return status.Error(codes.PermissionDenied, err.Error())
		}
	}
	_ = token
	return nil
}
