package delegatedsubject

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// AppendToOutgoingContext signs a delegated subject and attaches it to gRPC metadata.
func AppendToOutgoingContext(ctx context.Context, signer *Signer, in SignInput) (context.Context, error) {
	if signer == nil || !signer.Enabled() {
		return ctx, nil
	}
	token, err := signer.Sign(in)
	if err != nil {
		return ctx, err
	}
	return metadata.AppendToOutgoingContext(ctx, MetadataKey, token), nil
}

// FromIncomingContext reads and verifies a delegated-subject token from gRPC metadata.
func FromIncomingContext(ctx context.Context, verifier *Verifier, purpose string, testeeID uint64) (Token, error) {
	if verifier == nil || !verifier.Enabled() {
		return Token{}, nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return Token{}, ErrMissingToken
	}
	values := md.Get(MetadataKey)
	if len(values) == 0 || values[0] == "" {
		return Token{}, ErrMissingToken
	}
	return verifier.Verify(values[0], purpose, testeeID)
}
