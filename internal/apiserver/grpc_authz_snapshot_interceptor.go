package apiserver

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// gRPC IAM 认证拦截器写入的 context 键（见 internal/pkg/grpc/interceptor_auth.go injectUserContext）。
const (
	grpcCtxUserIDKey   = "user_id"
	grpcCtxTenantIDKey = "tenant_id"
)

// NewAuthzSnapshotUnaryInterceptor 在 IAM JWT 之后加载授权快照并写入 context，
// 与 HTTP 的 AuthzSnapshotMiddleware 对齐，供 TesteeAccessService、Capability 等使用。
func NewAuthzSnapshotUnaryInterceptor(loader *iam.AuthzSnapshotLoader) grpc.UnaryServerInterceptor {
	if loader == nil {
		return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			return handler(ctx, req)
		}
	}
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if grpcAuthzSnapshotSkipMethod(info.FullMethod) {
			return handler(ctx, req)
		}
		tenantID := grpcCtxString(ctx, grpcCtxTenantIDKey)
		userIDStr := grpcCtxString(ctx, grpcCtxUserIDKey)
		if tenantID == "" || userIDStr == "" {
			// 未走 IAM（如健康检查、内部免鉴权 RPC）或无租户/用户声明：不注入快照。
			return handler(ctx, req)
		}
		if _, err := strconv.ParseUint(tenantID, 10, 64); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "tenant_id must be a numeric organization id for QS")
		}
		snap, err := loader.Load(ctx, tenantID, userIDStr)
		if err != nil {
			return nil, status.Errorf(codes.Unavailable, "failed to load authorization snapshot: %v", err)
		}
		ctx = authz.WithSnapshot(ctx, snap)
		if uid, err := strconv.ParseUint(userIDStr, 10, 64); err == nil {
			ctx = actorctx.WithGrantingUserID(ctx, uid)
		}
		return handler(ctx, req)
	}
}

func grpcAuthzSnapshotSkipMethod(fullMethod string) bool {
	// 与 IAMAuthInterceptor.skipMethods 对齐，避免对健康/反射拉授权快照。
	switch fullMethod {
	case "/grpc.health.v1.Health/Check", "/grpc.health.v1.Health/Watch":
		return true
	}
	for _, prefix := range []string{
		"/grpc.reflection.v1alpha.ServerReflection/",
		"/grpc.reflection.v1.ServerReflection/",
	} {
		if strings.HasPrefix(fullMethod, prefix) {
			return true
		}
	}
	return false
}

func grpcCtxString(ctx context.Context, key string) string {
	if ctx == nil {
		return ""
	}
	v := ctx.Value(key)
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}
