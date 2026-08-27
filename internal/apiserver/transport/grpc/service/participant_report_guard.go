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
	if s == nil {
		return status.Error(codes.FailedPrecondition, "participant report service is not configured")
	}
	_, err := verifyDelegatedSubject(ctx, s.delegatedVerifier, testeeID, purpose, false)
	return err
}

func verifyDelegatedSubject(ctx context.Context, verifier *delegatedsubject.Verifier, testeeID uint64, purpose string, required bool) (delegatedsubject.Token, error) {
	if verifier == nil || !verifier.Enabled() {
		if required {
			return delegatedsubject.Token{}, status.Error(codes.FailedPrecondition, "delegated subject verification is required")
		}
		return delegatedsubject.Token{}, nil
	}
	if identity, ok := pkggrpc.ServiceIdentityFromMTLSContext(ctx); ok {
		if err := verifier.AllowWorkload(identity.ServiceID); err != nil {
			return delegatedsubject.Token{}, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	token, err := delegatedsubject.FromIncomingContext(ctx, verifier, purpose, testeeID)
	if err != nil {
		switch {
		case errors.Is(err, delegatedsubject.ErrMissingToken),
			errors.Is(err, delegatedsubject.ErrInvalidToken),
			errors.Is(err, delegatedsubject.ErrExpiredToken):
			return delegatedsubject.Token{}, status.Error(codes.Unauthenticated, err.Error())
		default:
			return delegatedsubject.Token{}, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	return token, nil
}
