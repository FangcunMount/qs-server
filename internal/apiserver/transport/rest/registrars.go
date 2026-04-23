package rest

import (
	"fmt"
	"net/http"

	auth "github.com/FangcunMount/iam-contracts/pkg/sdk/auth/verifier"
	codesHandler "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

type publicRouteRegistrar struct {
	router *Router
}

type protectedRouteRegistrar struct {
	router *Router
}

type internalRouteRegistrar struct {
	router *Router
}

type protectedGroupMiddlewareComposer struct {
	router *Router
}

func newPublicRouteRegistrar(router *Router) publicRouteRegistrar {
	return publicRouteRegistrar{router: router}
}

func newProtectedRouteRegistrar(router *Router) protectedRouteRegistrar {
	return protectedRouteRegistrar{router: router}
}

func newInternalRouteRegistrar(router *Router) internalRouteRegistrar {
	return internalRouteRegistrar{router: router}
}

func newProtectedGroupMiddlewareComposer(router *Router) protectedGroupMiddlewareComposer {
	return protectedGroupMiddlewareComposer{router: router}
}

func (registrar publicRouteRegistrar) register(engine *gin.Engine) {
	r := registrar.router
	engine.GET("/health", r.healthCheck)
	engine.GET("/readyz", r.readyCheck)
	engine.GET("/ping", r.ping)
	engine.GET("/governance/redis", r.redisGovernance)

	publicAPI := engine.Group("/api/v1/public")
	{
		publicAPI.GET("/info", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"service":     "questionnaire-scale",
				"version":     "1.0.0",
				"description": "问卷量表管理系统",
			})
		})
		r.registerActorPublicRoutes(publicAPI)
	}

	objectKeyPrefix := "qrcode"
	if r.deps.QRCodeObjectKeyPrefix != "" {
		objectKeyPrefix = r.deps.QRCodeObjectKeyPrefix
	}
	qrcodeHandler := codesHandler.NewQRCodeHandler(r.deps.QRCodeObjectStore, objectKeyPrefix)
	engine.GET("/api/v1/qrcodes/:filename", qrcodeHandler.GetQRCodeImage)
}

func (registrar protectedRouteRegistrar) register(engine *gin.Engine) {
	r := registrar.router
	apiV1 := engine.Group("/api/v1")
	r.applyProtectedGroupMiddlewares(apiV1, "/api/v1")

	r.registerUserProtectedRoutes(apiV1)
	r.registerQuestionnaireProtectedRoutes(apiV1)
	r.registerAnswersheetProtectedRoutes(apiV1)
	r.registerScaleProtectedRoutes(apiV1)
	r.registerEvaluationProtectedRoutes(apiV1)
	r.registerActorProtectedRoutes(apiV1)
	r.registerPlanProtectedRoutes(apiV1)
	r.registerStatisticsProtectedRoutes(apiV1)
	r.registerCodesRoutes(apiV1)
	r.registerAdminRoutes(apiV1)
}

func (registrar internalRouteRegistrar) register(engine *gin.Engine) {
	r := registrar.router
	internalV1 := engine.Group("/internal/v1")
	r.applyProtectedGroupMiddlewares(internalV1, "/internal/v1")

	r.registerPlanInternalRoutes(internalV1)
	r.registerStatisticsInternalRoutes(internalV1)
	r.registerCacheGovernanceInternalRoutes(internalV1)
}

func (composer protectedGroupMiddlewareComposer) apply(group *gin.RouterGroup, routePrefix string) {
	r := composer.router
	if r.deps.IAM.Enabled {
		tokenVerifier := r.deps.IAM.TokenVerifier
		if tokenVerifier != nil {
			verifyOpts := r.iamVerifyOptions()
			group.Use(middleware.JWTAuthMiddlewareWithOptions(tokenVerifier, verifyOpts))
			group.Use(restmiddleware.UserIdentityMiddleware())
			group.Use(restmiddleware.RequireTenantIDMiddleware())
			group.Use(restmiddleware.RequireNumericOrgScopeMiddleware())
			if r.deps.Actor.ActiveOperatorRepo != nil {
				group.Use(restmiddleware.RequireActiveOperatorMiddleware(r.deps.Actor.ActiveOperatorRepo))
			}
			if loader := r.deps.IAM.SnapshotLoader; loader != nil {
				group.Use(restmiddleware.AuthzSnapshotMiddleware(loader, r.deps.Actor.OperatorRoleProjectionUpdater))
			} else {
				fmt.Printf("⚠️  Warning: IAM AuthzSnapshotLoader unavailable (need gRPC); authorization snapshot disabled for %s\n", routePrefix)
			}
			fmt.Printf("🔐 JWT authentication middleware enabled for %s (%s)\n", routePrefix, r.iamVerificationMode())
			return
		}
		fmt.Printf("⚠️  Warning: TokenVerifier not available, JWT authentication disabled for %s!\n", routePrefix)
		return
	}

	fmt.Printf("⚠️  Warning: IAM authentication is disabled, routes are unprotected for %s!\n", routePrefix)
}

func (r *Router) registerPublicRoutes(engine *gin.Engine) {
	newPublicRouteRegistrar(r).register(engine)
}

func (r *Router) registerProtectedRoutes(engine *gin.Engine) {
	newProtectedRouteRegistrar(r).register(engine)
}

func (r *Router) registerInternalRoutes(engine *gin.Engine) {
	newInternalRouteRegistrar(r).register(engine)
}

func (r *Router) applyProtectedGroupMiddlewares(group *gin.RouterGroup, routePrefix string) {
	newProtectedGroupMiddlewareComposer(r).apply(group, routePrefix)
}

func (r *Router) iamVerifyOptions() *auth.VerifyOptions {
	return &auth.VerifyOptions{
		ForceRemote:     r != nil && r.deps.IAM.ForceRemoteVerification,
		IncludeMetadata: true,
	}
}

func (r *Router) iamVerificationMode() string {
	opts := r.iamVerifyOptions()
	if opts.ForceRemote {
		return "authoritative remote verification"
	}
	return "local JWKS verification"
}
