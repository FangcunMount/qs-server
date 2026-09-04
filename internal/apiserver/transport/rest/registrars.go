package rest

import (
	"net/http"

	authnv2 "github.com/FangcunMount/iam/v3/api/grpc/iam/authn/v2"
	auth "github.com/FangcunMount/iam/v3/pkg/sdk/auth/verifier"
	codesHandler "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/orgscope"
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
	engine.GET("/governance/cache", r.cacheGovernance)

	publicAPI := engine.Group("/api/v1/public")
	{
		publicAPI.GET("/info", codesHandler.PublicInfo)
		r.registerActorPublicRoutes(publicAPI)
	}

	objectKeyPrefix := "qrcode"
	if r.deps.QRCodeObjectKeyPrefix != "" {
		objectKeyPrefix = r.deps.QRCodeObjectKeyPrefix
	}
	qrcodeHandler := codesHandler.NewQRCodeHandler(r.deps.QRCodeObjectStore, objectKeyPrefix)
	engine.GET("/api/v1/qrcodes/:filename", qrcodeHandler.GetQRCodeImage)
	assessmentImageHandler := codesHandler.NewAssessmentImageHandler(r.deps.AssessmentAssetStore, r.deps.AssessmentAssetKeyPrefix)
	engine.GET("/api/v1/assessment-assets/typology/:model/:outcome/:filename", assessmentImageHandler.GetOutcomeImage)
}

func (registrar protectedRouteRegistrar) register(engine *gin.Engine) {
	r := registrar.router
	apiV1 := engine.Group("/api/v1")
	r.applyProtectedGroupMiddlewares(apiV1, "/api/v1")

	r.registerQuestionnaireProtectedRoutes(apiV1)
	r.registerAssessmentModelProtectedRoutes(apiV1)
	r.registerNormTableProtectedRoutes(apiV1)
	r.registerAnswersheetProtectedRoutes(apiV1)
	r.registerEvaluationProtectedRoutes(apiV1)
	r.registerInterpretationProtectedRoutes(apiV1)
	r.registerActorProtectedRoutes(apiV1)
	r.registerPlanProtectedRoutes(apiV1)
	r.registerCodesRoutes(apiV1)

	apiV2 := engine.Group("/api/v2")
	r.applyProtectedGroupMiddlewares(apiV2, "/api/v2")
	r.registerEvaluationOutcomeProtectedRoutes(apiV2)
	r.registerStatisticsProtectedRoutes(apiV2)
	r.registerPlanV2ProtectedRoutes(apiV2)
}

func (registrar internalRouteRegistrar) register(engine *gin.Engine) {
	r := registrar.router
	internalV1 := engine.Group("/internal/v1")
	r.applyProtectedGroupMiddlewares(internalV1, "/internal/v1")

	r.registerPlanInternalRoutes(internalV1)
	r.registerEventStatusInternalRoutes(internalV1)
	r.registerResilienceInternalRoutes(internalV1)
	r.registerSystemGovernanceInternalRoutes(internalV1)
	r.registerEvaluationRunInternalRoutes(internalV1)
	r.registerInterpretationInternalRoutes(internalV1)
	internalV2 := engine.Group("/internal/v2")
	r.applyProtectedGroupMiddlewares(internalV2, "/internal/v2")
	r.registerInterpretationInternalV2Routes(internalV2)
	r.registerStatisticsInternalRoutes(internalV2)
}

func (composer protectedGroupMiddlewareComposer) apply(group *gin.RouterGroup, routePrefix string) {
	r := composer.router
	if !r.deps.IAM.Enabled || r.deps.IAM.TokenVerifier == nil || r.deps.IAM.SnapshotLoader == nil {
		group.Use(unavailableAuthorizationMiddleware(routePrefix))
		return
	}

	verifyOpts := r.iamVerifyOptions()
	group.Use(middleware.JWTAuthMiddlewareWithOptions(r.deps.IAM.TokenVerifier, verifyOpts))
	group.Use(restmiddleware.UserIdentityMiddleware())
	group.Use(restmiddleware.RequireTenantDomainMiddleware())
	if r.deps.Actor.ActiveOperatorChecker != nil {
		group.Use(restmiddleware.ResolveOperatorOrgScopeMiddleware(r.deps.Actor.ActiveOperatorChecker))
	} else {
		group.Use(restmiddleware.ResolveOrgScopeMiddleware(orgscope.FixedResolver(orgscope.DefaultOrgID)))
	}
	group.Use(restmiddleware.RequireOrgScopeMiddleware())
	group.Use(restmiddleware.AuthzSnapshotMiddleware(r.deps.IAM.SnapshotLoader, r.deps.Actor.OperatorRoleProjectionUpdater))
}

func unavailableAuthorizationMiddleware(routePrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Allow an already-completed trusted middleware chain (used by composed
		// transports and route contract tests), but never infer authorization
		// from request headers. Production startup rejects missing IAM runtime.
		if restmiddleware.GetAuthzSnapshot(c) != nil &&
			restmiddleware.GetUserID(c) != 0 &&
			restmiddleware.GetTenantDomain(c) != "" &&
			restmiddleware.GetOrgID(c) != 0 {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
			"code":    "authorization_runtime_unavailable",
			"message": "authorization runtime is unavailable",
			"route":   routePrefix,
		})
	}
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
		ForceRemote:       r != nil && r.deps.IAM.ForceRemoteVerification,
		IncludeMetadata:   true,
		AllowedTokenTypes: []authnv2.TokenType{authnv2.TokenType_TOKEN_TYPE_ACCESS},
	}
}
