package grpc

import (
	"context"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/pkg/orgscope"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// NewOrgScopeUnaryInterceptor resolves QS business org_id after IAM JWT auth.
func NewOrgScopeUnaryInterceptor(resolve orgscope.ResolveFunc) grpc.UnaryServerInterceptor {
	if resolve == nil {
		resolve = orgscope.FixedResolver(orgscope.DefaultOrgID)
	}
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if shouldSkipOrgScopeMethod(info.FullMethod) {
			return handler(ctx, req)
		}
		userIDStr := UserIDFromContext(ctx)
		if userIDStr == "" {
			return handler(ctx, req)
		}
		userID, err := strconv.ParseUint(userIDStr, 10, 64)
		if err != nil || userID == 0 {
			return nil, status.Errorf(codes.Unauthenticated, "invalid user id in token")
		}
		requested := orgscope.RequestedOrgIDFromMetadata(incomingMetadataMap(ctx))
		orgID, err := resolve(ctx, userID, requested)
		if err != nil || orgID == 0 {
			return nil, orgscope.GRPCStatusForResolveError(err)
		}
		ctx = context.WithValue(ctx, authContextKeyOrgID, orgID)
		return handler(ctx, req)
	}
}

func shouldSkipOrgScopeMethod(fullMethod string) bool {
	switch fullMethod {
	case "/grpc.health.v1.Health/Check", "/grpc.health.v1.Health/Watch":
		return true
	}
	for _, prefix := range []string{
		"/grpc.reflection.v1alpha.ServerReflection/",
		"/grpc.reflection.v1.ServerReflection/",
	} {
		if len(fullMethod) >= len(prefix) && fullMethod[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func incomingMetadataMap(ctx context.Context) map[string]string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(md))
	for key, values := range md {
		if len(values) == 0 {
			continue
		}
		out[key] = values[0]
	}
	return out
}
