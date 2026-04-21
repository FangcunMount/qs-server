package apiserver

import (
	"fmt"
	"net/http"
	"time"

	"github.com/FangcunMount/iam-contracts/pkg/sdk/auth"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	domainoperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	codesHandler "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

// Router 集中的路由管理器
type Router struct {
	container *container.Container
	rateCfg   *options.RateLimitOptions
}

// NewRouter 创建路由管理器
func NewRouter(c *container.Container, rateCfg *options.RateLimitOptions) *Router {
	if rateCfg == nil {
		rateCfg = options.NewRateLimitOptions()
	}

	return &Router{
		container: c,
		rateCfg:   rateCfg,
	}
}

// RegisterRoutes 注册所有路由
func (r *Router) RegisterRoutes(engine *gin.Engine) {
	// OpenAPI 契约（OAS 3.1）与 UI
	engine.Static("/api/rest", "./api/rest")
	engine.Static("/swagger-ui", "./web/swagger-ui/swagger-ui-dist")
	// 兼容入口
	engine.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/swagger-ui/")
	})

	// 注册公开路由（不需要认证）
	r.registerPublicRoutes(engine)

	// 注册需要认证的路由
	r.registerProtectedRoutes(engine)
	r.registerInternalRoutes(engine)

	fmt.Printf("🔗 Registered routes for: public, protected(api/v1), internal(internal/v1)\n")
}

// registerPublicRoutes 注册公开路由（不需要认证）
func (r *Router) registerPublicRoutes(engine *gin.Engine) {
	// 健康检查和基础路由
	engine.GET("/health", r.healthCheck)
	engine.GET("/readyz", r.readyCheck)
	engine.GET("/ping", r.ping)
	engine.GET("/governance/redis", r.redisGovernance)

	// 认证相关的公开路由 已迁移至 IAM / API 网关，不在此维护

	// 公开的API路由
	publicAPI := engine.Group("/api/v1/public")
	{
		publicAPI.GET("/info", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"service":     "questionnaire-scale",
				"version":     "1.0.0",
				"description": "问卷量表管理系统",
			})
		})

		if r.container != nil && r.container.ActorModule != nil && r.container.ActorModule.ActorHandler != nil {
			publicAPI.GET("/assessment-entries/:token", r.container.ActorModule.ActorHandler.ResolveAssessmentEntry)
			publicAPI.POST("/assessment-entries/:token/intake", r.container.ActorModule.ActorHandler.IntakeAssessmentEntry)
		}
	}

	// 二维码图片访问路由（公开，不需要认证）
	qrcodeHandler := codesHandler.NewQRCodeHandler()
	engine.GET("/api/v1/qrcodes/:filename", qrcodeHandler.GetQRCodeImage)
}

// registerProtectedRoutes 注册需要认证的路由
func (r *Router) registerProtectedRoutes(engine *gin.Engine) {
	// 创建需要认证的API组
	apiV1 := engine.Group("/api/v1")
	r.applyProtectedGroupMiddlewares(apiV1, "/api/v1")

	// 注册用户相关的受保护路由
	r.registerUserProtectedRoutes(apiV1)

	// 注册问卷相关的受保护路由
	r.registerQuestionnaireProtectedRoutes(apiV1)

	// 注册答卷相关的受保护路由
	r.registerAnswersheetProtectedRoutes(apiV1)

	// 注册量表相关的受保护路由
	r.registerScaleProtectedRoutes(apiV1)

	// 注册 Evaluation 模块相关的受保护路由
	r.registerEvaluationProtectedRoutes(apiV1)

	// 注册 Plan 模块相关的受保护路由（必须在 registerActorProtectedRoutes 之前，确保更具体的路由先注册）
	r.registerPlanProtectedRoutes(apiV1)

	// 注册 Statistics 模块相关的受保护路由
	r.registerStatisticsProtectedRoutes(apiV1)

	// 注册 Actor 模块相关的受保护路由
	r.registerActorProtectedRoutes(apiV1)

	// 注册 Codes 申请路由
	r.registerCodesRoutes(apiV1)

	// 管理员路由（需要额外的权限检查）
	r.registerAdminRoutes(apiV1)
}

func (r *Router) registerInternalRoutes(engine *gin.Engine) {
	internalV1 := engine.Group("/internal/v1")
	r.applyProtectedGroupMiddlewares(internalV1, "/internal/v1")

	r.registerPlanInternalRoutes(internalV1)
	r.registerStatisticsInternalRoutes(internalV1)
	r.registerCacheGovernanceInternalRoutes(internalV1)
}

func (r *Router) applyProtectedGroupMiddlewares(group *gin.RouterGroup, routePrefix string) {
	if r.container.IAMModule != nil && r.container.IAMModule.IsEnabled() {
		tokenVerifier := r.container.IAMModule.SDKTokenVerifier()
		if tokenVerifier != nil {
			verifyOpts := r.iamVerifyOptions()
			group.Use(middleware.JWTAuthMiddlewareWithOptions(tokenVerifier, verifyOpts))
			group.Use(restmiddleware.UserIdentityMiddleware())
			group.Use(restmiddleware.RequireTenantIDMiddleware())
			group.Use(restmiddleware.RequireNumericOrgScopeMiddleware())
			if r.container.ActorModule != nil && r.container.ActorModule.OperatorRepo != nil {
				group.Use(restmiddleware.RequireActiveOperatorMiddleware(r.container.ActorModule.OperatorRepo))
			}
			if loader := r.container.IAMModule.AuthzSnapshotLoader(); loader != nil {
				// 授权快照只负责权限视图，不替代 JWT 的权威在线校验。
				var operatorRepo domainoperator.Repository
				if r.container.ActorModule != nil {
					operatorRepo = r.container.ActorModule.OperatorRepo
				}
				group.Use(restmiddleware.AuthzSnapshotMiddleware(loader, operatorRepo))
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

func (r *Router) iamVerifyOptions() *auth.VerifyOptions {
	if r == nil || r.container == nil || r.container.IAMModule == nil || r.container.IAMModule.Client() == nil {
		return &auth.VerifyOptions{IncludeMetadata: true}
	}
	cfg := r.container.IAMModule.Client().Config()
	if cfg == nil || cfg.JWT == nil {
		return &auth.VerifyOptions{IncludeMetadata: true}
	}
	return &auth.VerifyOptions{
		ForceRemote:     cfg.JWT.ForceRemoteVerification,
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

// registerUserProtectedRoutes 注册用户相关的受保护路由
// 用户管理已迁移到 IAM 服务，此方法保留以便未来扩展
func (r *Router) registerUserProtectedRoutes(apiV1 *gin.RouterGroup) {
	// 用户相关功能已迁移到 iam-contracts 项目
}

// registerCodesRoutes 注册 codes 申请路由
func (r *Router) registerCodesRoutes(apiV1 *gin.RouterGroup) {
	if r.container == nil {
		return
	}

	if r.container.CodesService == nil {
		return
	}

	handler := codesHandler.NewCodesHandler(r.container.CodesService)
	codes := apiV1.Group("/codes", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	codes.POST("/apply", handler.Apply)
}

// registerQuestionnaireProtectedRoutes 注册问卷相关的受保护路由
func (r *Router) registerQuestionnaireProtectedRoutes(apiV1 *gin.RouterGroup) {
	quesHandler := r.container.SurveyModule.Questionnaire.Handler
	if quesHandler == nil {
		return
	}

	questionnaires := apiV1.Group("/questionnaires")
	{
		manage := questionnaires.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageQuestionnaires))
		read := questionnaires.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityReadQuestionnaires))

		// 生命周期管理
		manage.POST("", quesHandler.Create)                          // 创建问卷
		manage.PUT("/:code/basic-info", quesHandler.UpdateBasicInfo) // 更新基本信息
		manage.POST("/:code/draft", quesHandler.SaveDraft)           // 保存草稿
		manage.POST("/:code/publish", quesHandler.Publish)           // 发布问卷
		manage.POST("/:code/unpublish", quesHandler.Unpublish)       // 取消发布
		manage.POST("/:code/archive", quesHandler.Archive)           // 归档问卷
		manage.DELETE("/:code", quesHandler.Delete)                  // 删除问卷

		// 问题内容管理
		manage.POST("/:code/questions", quesHandler.AddQuestion)               // 添加问题
		manage.PUT("/:code/questions/:qcode", quesHandler.UpdateQuestion)      // 更新问题
		manage.DELETE("/:code/questions/:qcode", quesHandler.RemoveQuestion)   // 删除问题
		manage.POST("/:code/questions/reorder", quesHandler.ReorderQuestions)  // 重排问题
		manage.PUT("/:code/questions/batch", quesHandler.BatchUpdateQuestions) // 批量更新

		// 查询接口
		read.GET("", quesHandler.List)                               // 获取问卷列表
		read.GET("/:code", quesHandler.GetByCode)                    // 获取问卷详情
		read.GET("/published/:code", quesHandler.GetPublishedByCode) // 获取已发布问卷
		read.GET("/published", quesHandler.ListPublished)            // 获取已发布列表
		read.GET("/:code/qrcode", quesHandler.GetQRCode)             // 获取问卷小程序码
	}
}

// registerAnswersheetProtectedRoutes 注册答卷相关的受保护路由
func (r *Router) registerAnswersheetProtectedRoutes(apiV1 *gin.RouterGroup) {
	answersheetHandler := r.container.SurveyModule.AnswerSheet.Handler
	if answersheetHandler == nil {
		return
	}

	answersheets := apiV1.Group("/answersheets")
	{
		admin := answersheets.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		read := answersheets.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityReadAnswersheets))

		// 管理接口
		admin.POST("/admin-submit", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.AdminSubmitGlobalQPS,
			r.rateCfg.AdminSubmitGlobalBurst,
			r.rateCfg.AdminSubmitUserQPS,
			r.rateCfg.AdminSubmitUserBurst,
			answersheetHandler.AdminSubmit,
		)...)
		read.GET("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			answersheetHandler.GetByID,
		)...)
		read.GET("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			answersheetHandler.List,
		)...)
		// 统计接口已迁移到 /api/v1/statistics/questionnaires/:code
	}
}

// registerScaleProtectedRoutes 注册量表相关的受保护路由
func (r *Router) registerScaleProtectedRoutes(apiV1 *gin.RouterGroup) {
	scaleHandler := r.container.ScaleModule.Handler
	if scaleHandler == nil {
		return
	}

	scales := apiV1.Group("/scales")
	{
		manage := scales.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageScales))
		read := scales.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityReadScales))

		// 生命周期管理
		manage.POST("", scaleHandler.Create)                                 // 创建量表
		manage.PUT("/:code/basic-info", scaleHandler.UpdateBasicInfo)        // 更新基本信息
		manage.PUT("/:code/questionnaire", scaleHandler.UpdateQuestionnaire) // 更新关联问卷
		manage.POST("/:code/publish", scaleHandler.Publish)                  // 发布量表
		manage.POST("/:code/unpublish", scaleHandler.Unpublish)              // 下架量表
		manage.POST("/:code/archive", scaleHandler.Archive)                  // 归档量表
		manage.DELETE("/:code", scaleHandler.Delete)                         // 删除量表

		// 因子管理（仅提供批量操作）
		manage.PUT("/:code/factors/batch", scaleHandler.BatchUpdateFactors)      // 批量更新因子
		manage.PUT("/:code/interpret-rules", scaleHandler.ReplaceInterpretRules) // 批量设置解读规则

		// 查询接口（注意：具体路径要放在参数路径之前，避免路由冲突）
		read.GET("/categories", scaleHandler.GetCategories)                // 获取量表分类列表
		read.GET("/by-questionnaire", scaleHandler.GetByQuestionnaireCode) // 根据问卷获取量表
		read.GET("/published/:code", scaleHandler.GetPublishedByCode)      // 获取已发布量表
		read.GET("/published", scaleHandler.ListPublished)                 // 获取已发布列表
		read.GET("/:code/factors", scaleHandler.GetFactors)                // 获取因子列表
		read.GET("/:code/qrcode", scaleHandler.GetQRCode)                  // 获取量表小程序码
		read.GET("/:code", scaleHandler.GetByCode)                         // 获取量表详情
		read.GET("", scaleHandler.List)                                    // 获取量表列表
	}
}

// registerActorProtectedRoutes 注册 Actor 模块相关的受保护路由
func (r *Router) registerActorProtectedRoutes(apiV1 *gin.RouterGroup) {
	actorHandler := r.container.ActorModule.ActorHandler
	if actorHandler == nil {
		return
	}

	// 受试者路由
	testees := apiV1.Group("/testees")
	{
		testees.GET("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.ListTestees,
		)...)
		testees.GET("/by-profile-id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.GetTesteeByProfileID,
		)...)
		testees.GET("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.GetTestee,
		)...)
		testees.PUT("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.UpdateTestee,
		)...)
		testees.GET("/:id/scale-analysis", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.GetScaleAnalysis,
		)...)
		// 统计接口已迁移到 /api/v1/statistics/testees/:testee_id
	}

	// 员工路由
	staff := apiV1.Group("/staff", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	{
		staff.POST("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.CreateStaff,
		)...)
		staff.GET("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.ListStaff,
		)...)
		staff.GET("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.GetStaff,
		)...)
		staff.PUT("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.UpdateStaff,
		)...)
		staff.DELETE("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.DeleteStaff,
		)...)
	}

	registerClinicianRoutes := func(group *gin.RouterGroup) {
		adminClinicians := group.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		adminClinicians.POST("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.CreateClinician,
		)...)
		adminClinicians.GET("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.ListClinicians,
		)...)
		adminClinicians.PUT("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.UpdateClinician,
		)...)
		adminClinicians.POST("/:id/activate", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.ActivateClinician,
		)...)
		adminClinicians.POST("/:id/deactivate", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.DeactivateClinician,
		)...)
		adminClinicians.POST("/:id/bind-operator", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.BindClinicianOperator,
		)...)
		adminClinicians.POST("/:id/unbind-operator", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.UnbindClinicianOperator,
		)...)
		me := group.Group("/me")
		me.GET("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.GetMyClinician,
		)...)
		me.GET("/testees", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.ListMyClinicianTestees,
		)...)
		me.GET("/relations", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.ListMyClinicianRelations,
		)...)
		me.POST("/assessment-entries", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.CreateMyAssessmentEntry,
		)...)
		me.GET("/assessment-entries", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.ListMyAssessmentEntries,
		)...)
		me.GET("/assessment-entries/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.GetMyAssessmentEntry,
		)...)
		me.POST("/assessment-entries/:id/deactivate", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.DeactivateMyAssessmentEntry,
		)...)
		me.POST("/assessment-entries/:id/reactivate", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.ReactivateMyAssessmentEntry,
		)...)
		adminClinicians.GET("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.GetClinician,
		)...)
		adminClinicians.GET("/:id/testees", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.ListClinicianTestees,
		)...)
		adminClinicians.GET("/:id/relations", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.ListClinicianRelations,
		)...)
		adminClinicians.POST("/:id/assessment-entries", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.CreateClinicianAssessmentEntry,
		)...)
		adminClinicians.GET("/:id/assessment-entries", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.ListClinicianAssessmentEntries,
		)...)
	}

	clinicians := apiV1.Group("/clinicians")
	registerClinicianRoutes(clinicians)

	// 兼容旧的 /practitioners 路由，后续客户端切换完成后可移除。
	practitioners := apiV1.Group("/practitioners")
	registerClinicianRoutes(practitioners)

	relationAdmin := apiV1.Group("/clinician-testee-relations", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	{
		relationAdmin.POST("/assign", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.AssignClinicianTestee,
		)...)
		relationAdmin.POST("/assign-primary", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.AssignPrimaryClinicianTestee,
		)...)
		relationAdmin.POST("/assign-attending", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.AssignAttendingClinicianTestee,
		)...)
		relationAdmin.POST("/assign-collaborator", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.AssignCollaboratorClinicianTestee,
		)...)
		relationAdmin.POST("/transfer-primary", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.TransferPrimaryClinicianTestee,
		)...)
		relationAdmin.POST("/:id/unbind", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.UnbindClinicianTesteeRelation,
		)...)
	}

	assessmentEntries := apiV1.Group("/assessment-entries", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	{
		assessmentEntries.GET("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			actorHandler.GetAssessmentEntry,
		)...)
		assessmentEntries.POST("/:id/deactivate", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.DeactivateAssessmentEntry,
		)...)
		assessmentEntries.POST("/:id/reactivate", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			actorHandler.ReactivateAssessmentEntry,
		)...)
	}

	testees.GET("/:id/clinicians", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		actorHandler.GetTesteeClinicians,
	)...)
	testees.GET("/:id/clinician-relations", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		actorHandler.ListTesteeClinicianRelations,
	)...)
}

// registerEvaluationProtectedRoutes 注册评估模块相关的受保护路由
func (r *Router) registerEvaluationProtectedRoutes(apiV1 *gin.RouterGroup) {
	evalHandler := r.container.EvaluationModule.Handler
	if evalHandler == nil {
		return
	}

	evaluations := apiV1.Group("/evaluations")
	{
		// ==================== Assessment 路由 =====================
		assessments := evaluations.Group("/assessments")
		{
			// 查询
			assessments.GET("", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.ListAssessments,
			)...)
			assessments.GET("/:id", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetAssessment,
			)...)
			// 统计接口已迁移到 /api/v1/statistics/questionnaires/:code 或 /api/v1/statistics/system

			// 得分和报告
			assessments.GET("/:id/scores", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetScores,
			)...)
			assessments.GET("/:id/report", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetReport,
			)...)
			assessments.GET("/:id/high-risk-factors", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetHighRiskFactors,
			)...)
			// 管理操作
			assessmentAdmin := assessments.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityEvaluateAssessments))
			assessmentAdmin.POST("/:id/retry", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				evalHandler.RetryFailed,
			)...)
		}

		// ==================== Score 相关路由 ====================
		scores := evaluations.Group("/scores")
		{
			scores.GET("/trend", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetFactorTrend,
			)...)
		}

		// ==================== Report 相关路由 ====================
		reports := evaluations.Group("/reports")
		{
			reports.GET("", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.ListReports,
			)...)
		}

		// ==================== 批量操作路由 ====================
		evaluationAdmin := evaluations.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityEvaluateAssessments))
		evaluationAdmin.POST("/batch-evaluate", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			evalHandler.BatchEvaluate,
		)...)
	}
}

func (r *Router) rateLimitedHandlers(
	rateCfg *options.RateLimitOptions,
	globalQPS float64,
	globalBurst int,
	userQPS float64,
	userBurst int,
	handler gin.HandlerFunc,
) []gin.HandlerFunc {
	if !rateCfg.Enabled {
		return []gin.HandlerFunc{handler}
	}

	return []gin.HandlerFunc{
		middleware.Limit(globalQPS, globalBurst),
		middleware.LimitByKey(userQPS, userBurst, requestLimitKey),
		handler,
	}
}

func requestLimitKey(c *gin.Context) string {
	userID := middleware.GetUserID(c)
	if userID != "" {
		return "user:" + userID
	}
	return "ip:" + c.ClientIP()
}

// registerPlanProtectedRoutes 注册 Plan 模块相关的受保护路由
func (r *Router) registerPlanProtectedRoutes(apiV1 *gin.RouterGroup) {
	planHandler := r.container.PlanModule.Handler
	if planHandler == nil {
		return
	}

	plans := apiV1.Group("/plans")
	{
		// ==================== Plan 生命周期管理 ====================
		planWrites := plans.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageEvaluationPlans))
		planWrites.POST("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.CreatePlan,
		)...)
		planWrites.POST("/:id/pause", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.PausePlan,
		)...)
		planWrites.POST("/:id/resume", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.ResumePlan,
		)...)
		planWrites.POST("/:id/finish", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.FinishPlan,
		)...)
		planWrites.POST("/:id/cancel", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.CancelPlan,
		)...)

		// ==================== Plan 查询 ====================
		plans.GET("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.ListPlans,
		)...)
		plans.GET("/:id/tasks", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.ListTasksByPlan,
		)...)
		plans.GET("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.GetPlan,
		)...)

		// ==================== Plan 受试者管理 ====================
		planWrites.POST("/enroll", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.EnrollTestee,
		)...)
		planWrites.POST("/:id/testees/:testee_id/terminate", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.TerminateEnrollment,
		)...)
	}

	// ==================== Task 管理 ====================
	tasks := apiV1.Group("/plans/tasks")
	{
		taskWrites := tasks.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageEvaluationPlans))
		tasks.GET("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.ListTasks,
		)...)
		tasks.GET("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.GetTask,
		)...)
		taskWrites.POST("/:id/open", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.OpenTask,
		)...)
		taskWrites.POST("/:id/cancel", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.CancelTask,
		)...)
	}

	// ==================== Testee 相关的 Plan 查询 ====================
	// 注意：这些路由必须在 registerActorProtectedRoutes 之后注册，且更具体的路由要放在前面
	testees := apiV1.Group("/testees")
	{
		testees.GET("/:id/plans/:plan_id/tasks", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.ListTasksByTesteeAndPlan,
		)...)
		testees.GET("/:id/plans", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.ListPlansByTestee,
		)...)
		testees.GET("/:id/tasks", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.ListTasksByTestee,
		)...)
	}
}

// registerStatisticsProtectedRoutes 注册 Statistics 模块相关的受保护路由
func (r *Router) registerStatisticsProtectedRoutes(apiV1 *gin.RouterGroup) {
	statisticsModule := r.container.StatisticsModule
	if statisticsModule == nil || statisticsModule.Handler == nil {
		return
	}

	statistics := apiV1.Group("/statistics")
	{
		// ==================== 统计查询 ====================
		adminStatistics := statistics.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		adminStatistics.GET("/overview", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetOverview,
		)...)
		adminStatistics.GET("/clinicians", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.ListClinicianStatistics,
		)...)
		adminStatistics.GET("/clinicians/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetClinicianStatistics,
		)...)
		adminStatistics.GET("/entries", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.ListAssessmentEntryStatistics,
		)...)
		adminStatistics.GET("/entries/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetAssessmentEntryStatistics,
		)...)
		adminStatistics.GET("/system", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetSystemStatistics,
		)...)
		adminStatistics.GET("/questionnaires/:code", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetQuestionnaireStatistics,
		)...)
		statistics.GET("/testees/:testee_id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetTesteeStatistics,
		)...)
		statistics.GET("/testees/:testee_id/periodic", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetTesteePeriodicStatistics,
		)...)
		adminStatistics.GET("/plans/:plan_id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetPlanStatistics,
		)...)
		clinicianStatistics := statistics.Group("/clinicians/me")
		clinicianStatistics.GET("/overview", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetCurrentClinicianOverview,
		)...)
		clinicianStatistics.GET("/entries", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.ListCurrentClinicianEntryStatistics,
		)...)
		clinicianStatistics.GET("/testees-summary", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetCurrentClinicianTesteeSummary,
		)...)
		contentStatistics := statistics.Group("", restmiddleware.RequireAnyCapabilityMiddleware(
			restmiddleware.CapabilityManageQuestionnaires,
			restmiddleware.CapabilityManageScales,
		))
		contentStatistics.POST("/questionnaires/batch", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			statisticsModule.Handler.BatchQuestionnaireStatistics,
		)...)

		// ==================== 定时任务接口 ====================
	}
}

func (r *Router) registerPlanInternalRoutes(internalV1 *gin.RouterGroup) {
	planHandler := r.container.PlanModule.Handler
	if planHandler == nil {
		return
	}

	tasks := internalV1.Group("/plans/tasks", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageEvaluationPlans))
	tasks.POST("/schedule", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		planHandler.SchedulePendingTasks,
	)...)
	tasks.POST("/window", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		planHandler.ListTaskWindow,
	)...)
	tasks.POST("/:id/complete", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		planHandler.CompleteTask,
	)...)
	tasks.POST("/:id/expire", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		planHandler.ExpireTask,
	)...)
}

func (r *Router) registerStatisticsInternalRoutes(internalV1 *gin.RouterGroup) {
	statisticsModule := r.container.StatisticsModule
	if statisticsModule == nil || statisticsModule.Handler == nil {
		return
	}

	statistics := internalV1.Group("/statistics", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	sync := statistics.Group("/sync")
	sync.POST("/daily", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsModule.Handler.SyncDailyStatistics,
	)...)
	sync.POST("/accumulated", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsModule.Handler.SyncAccumulatedStatistics,
	)...)
	sync.POST("/plan", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsModule.Handler.SyncPlanStatistics,
	)...)
}

func (r *Router) registerCacheGovernanceInternalRoutes(internalV1 *gin.RouterGroup) {
	statisticsModule := r.container.StatisticsModule
	if statisticsModule == nil || statisticsModule.Handler == nil {
		return
	}

	governance := internalV1.Group("/cache/governance", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	governance.POST("/repair-complete", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsModule.Handler.RepairComplete,
	)...)
	governance.POST("/warmup-targets", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsModule.Handler.WarmupTargets,
	)...)
	governance.GET("/status", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		statisticsModule.Handler.CacheGovernanceStatus,
	)...)
	governance.GET("/hotset", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		statisticsModule.Handler.CacheGovernanceHotset,
	)...)
}

// registerAdminRoutes 注册管理员路由
func (r *Router) registerAdminRoutes(apiV1 *gin.RouterGroup) {
	admin := apiV1.Group("/admin")
	// admin.Use(r.requireAdminRole()) // 需要实现管理员权限检查中间件
	{
		admin.GET("/users", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			r.unsupportedFeature,
		)...)
		admin.GET("/statistics", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			r.unsupportedFeature,
		)...)
		admin.GET("/logs", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			r.unsupportedFeature,
		)...)
	}
}

// unsupportedFeature 明确标识当前保留但未支持的入口。
func (r *Router) unsupportedFeature(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    501,
		"message": "功能当前不支持",
		"path":    c.Request.URL.Path,
		"method":  c.Request.Method,
	})
}

// healthCheck 健康检查处理函数
// @Summary 健康检查
// @Description 检查 API Server 健康状态
// @Tags 系统
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /health [get]
func (r *Router) healthCheck(c *gin.Context) {
	response := gin.H{
		"status":       "healthy",
		"version":      "1.0.0",
		"discovery":    "auto",
		"architecture": "hexagonal",
		"router":       "centralized",
		"auth":         "delegated", // 认证由 IAM / API 网关代理
		"components": gin.H{
			"domain":      "questionnaire",
			"ports":       "storage",
			"adapters":    "mysql, mongodb, http",
			"application": "questionnaire_service",
		},
		// JWT 配置移除（由 IAM 管理）
	}

	c.JSON(200, response)
}

// ping 简单的连通性测试
// @Summary Ping
// @Description 测试 API Server 连通性
// @Tags 系统
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /ping [get]
func (r *Router) ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
		"status":  "ok",
		"router":  "centralized",
		"auth":    "enabled",
	})
}

func (r *Router) readyCheck(c *gin.Context) {
	snapshot := r.runtimeSnapshot(c)
	statusCode := http.StatusOK
	statusText := "ready"
	if !snapshot.Summary.Ready {
		statusCode = http.StatusServiceUnavailable
		statusText = "degraded"
	}
	c.JSON(statusCode, gin.H{
		"status":    statusText,
		"component": "apiserver",
		"redis":     snapshot,
	})
}

func (r *Router) redisGovernance(c *gin.Context) {
	c.JSON(http.StatusOK, r.runtimeSnapshot(c))
}

func (r *Router) runtimeSnapshot(c *gin.Context) cacheobservability.RuntimeSnapshot {
	if r != nil && r.container != nil && r.container.CacheGovernanceStatusService != nil {
		snapshot, err := r.container.CacheGovernanceStatusService.GetRuntime(c.Request.Context())
		if err == nil && snapshot != nil {
			return *snapshot
		}
	}
	return cacheobservability.RuntimeSnapshot{
		GeneratedAt: time.Now(),
		Component:   "apiserver",
		Families:    []cacheobservability.FamilyStatus{},
		Summary: cacheobservability.RuntimeSummary{
			Ready: true,
		},
	}
}
