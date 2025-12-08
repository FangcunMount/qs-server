package grpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/FangcunMount/iam-contracts/pkg/sdk/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// IAMAuthInterceptor IAM 认证拦截器
// 使用 SDK 的 TokenVerifier 进行本地 JWKS 验签（优先）或远程降级
type IAMAuthInterceptor struct {
	verifier    *auth.TokenVerifier // SDK TokenVerifier（支持本地验签）
	enabled     bool
	skipMethods map[string]bool // 跳过认证的方法列表
	requiremTLS bool            // 是否同时要求 mTLS
}

// NewIAMAuthInterceptor 创建 IAM 认证拦截器
// 接受 SDK 的 TokenVerifier，支持本地 JWKS 验签和远程降级
func NewIAMAuthInterceptor(verifier *auth.TokenVerifier, config *AuthConfig) *IAMAuthInterceptor {
	// 默认跳过健康检查和反射服务
	skipMethods := map[string]bool{
		"/grpc.health.v1.Health/Check":               true,
		"/grpc.health.v1.Health/Watch":               true,
		"/grpc.reflection.v1alpha.ServerReflection/": true, // 前缀匹配
		"/grpc.reflection.v1.ServerReflection/":      true,
	}

	return &IAMAuthInterceptor{
		verifier:    verifier,
		enabled:     config.Enabled,
		skipMethods: skipMethods,
		requiremTLS: config.RequireIdentityMatch,
	}
}

// UnaryServerInterceptor 返回一元拦截器
func (i *IAMAuthInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if !i.enabled {
			return handler(ctx, req)
		}

		// 检查是否跳过认证
		if i.shouldSkip(info.FullMethod) {
			return handler(ctx, req)
		}

		// 检查 verifier 是否可用
		if i.verifier == nil {
			return nil, status.Errorf(codes.Internal, "token verifier not configured")
		}

		// 1. 提取 JWT Token
		token, err := i.extractToken(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "missing or invalid authorization token: %v", err)
		}

		// 2. 使用 SDK TokenVerifier 验证（本地 JWKS 优先，远程降级）
		result, err := i.verifier.Verify(ctx, token, nil)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "token verification failed: %v", err)
		}

		if !result.Valid {
			return nil, status.Errorf(codes.Unauthenticated, "invalid token")
		}

		claims := result.Claims
		if claims == nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid token claims")
		}

		// 3. 如果启用 mTLS，验证身份一致性
		if i.requiremTLS {
			if err := i.verifyIdentityMatch(ctx, claims); err != nil {
				return nil, status.Errorf(codes.PermissionDenied, "identity mismatch: %v", err)
			}
		}

		// 4. 将用户信息注入 context
		ctx = i.injectUserContext(ctx, claims)

		// 5. 调用下一个处理器
		return handler(ctx, req)
	}
}

// shouldSkip 检查是否应该跳过认证
func (i *IAMAuthInterceptor) shouldSkip(fullMethod string) bool {
	// 精确匹配
	if i.skipMethods[fullMethod] {
		return true
	}

	// 前缀匹配（用于反射服务）
	for prefix := range i.skipMethods {
		if strings.HasSuffix(prefix, "/") && strings.HasPrefix(fullMethod, prefix) {
			return true
		}
	}

	return false
}

// extractToken 从 metadata 中提取 JWT token
func (i *IAMAuthInterceptor) extractToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", fmt.Errorf("no metadata found")
	}

	// 尝试从 authorization header 提取
	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return "", fmt.Errorf("authorization header not found")
	}

	authHeader := authHeaders[0]

	// 支持 "Bearer <token>" 格式
	const bearerPrefix = "Bearer "
	if strings.HasPrefix(authHeader, bearerPrefix) {
		return strings.TrimPrefix(authHeader, bearerPrefix), nil
	}

	// 直接使用 token（向后兼容）
	return authHeader, nil
}

// verifyIdentityMatch 验证 JWT 声明的身份与 mTLS 证书身份是否一致
func (i *IAMAuthInterceptor) verifyIdentityMatch(ctx context.Context, claims *auth.TokenClaims) error {
	// 从 context 中获取 mTLS 身份信息（由 MTLSInterceptor 注入）
	mtlsIdentity, ok := ctx.Value("mtls.identity").(map[string]interface{})
	if !ok {
		return fmt.Errorf("mTLS identity not found in context")
	}

	// 获取客户端 CN
	clientCN, ok := mtlsIdentity["common_name"].(string)
	if !ok || clientCN == "" {
		return fmt.Errorf("client CN not found in mTLS identity")
	}

	// 验证 JWT 中的 service_id 与证书 CN 是否匹配
	// 例如：证书 CN 为 "qs-collection.svc"，JWT service_id 应该是 "qs-collection"
	expectedServiceID := strings.TrimSuffix(clientCN, ".svc")

	// 从 claims 的 Extra 中获取 service_id（如果是服务间调用）
	if claims.Extra != nil {
		if serviceID, ok := claims.Extra["service_id"].(string); ok {
			if serviceID != expectedServiceID {
				return fmt.Errorf("service_id mismatch: JWT=%s, mTLS CN=%s", serviceID, clientCN)
			}
		}
	}

	return nil
}

// injectUserContext 将用户信息注入 context
func (i *IAMAuthInterceptor) injectUserContext(ctx context.Context, claims *auth.TokenClaims) context.Context {
	// 注入用户信息到 context，供后续业务逻辑使用
	ctx = context.WithValue(ctx, "user_id", claims.UserID)
	ctx = context.WithValue(ctx, "tenant_id", claims.TenantID)

	// 注入角色
	if len(claims.Roles) > 0 {
		ctx = context.WithValue(ctx, "roles", claims.Roles)
	}

	// 注入自定义声明（Extra）
	if claims.Extra != nil {
		ctx = context.WithValue(ctx, "custom_claims", claims.Extra)
		// 从 Extra 中提取 username（如果存在）
		if username, ok := claims.Extra["username"].(string); ok {
			ctx = context.WithValue(ctx, "username", username)
		}
	}

	return ctx
}

// AddSkipMethod 添加跳过认证的方法
func (i *IAMAuthInterceptor) AddSkipMethod(fullMethod string) {
	i.skipMethods[fullMethod] = true
}

// RemoveSkipMethod 移除跳过认证的方法
func (i *IAMAuthInterceptor) RemoveSkipMethod(fullMethod string) {
	delete(i.skipMethods, fullMethod)
}
