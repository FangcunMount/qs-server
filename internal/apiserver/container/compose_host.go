package container

import (
	"fmt"

	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	modelcatalogApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	modelcatalogRuntime "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/runtime"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	actormod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/actor"
	evalmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	reportmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/interpretation"
	ammod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/modelcatalog"
	planmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/plan"
	platformmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	statmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/workbenchreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

func (c *Container) RegisterModule(name string, module modules.Module) {
	c.registerModule(name, module)
}

func (c *Container) Printf(format string, args ...any) {
	c.printf(format, args...)
}

func (c *Container) MySQLDB() *gorm.DB { return c.mysqlDB }

func (c *Container) MongoDB() *mongo.Database { return c.mongoDB }

func (c *Container) RedisCache() redis.UniversalClient { return c.redisCache }

func (c *Container) EventPublisher() event.EventPublisher { return c.eventPublisher }

func (c *Container) EventProfile(profile eventcatalog.OutboxProfile) appEventing.ProfileBinding {
	if c == nil || c.eventSubsystem == nil {
		return appEventing.ProfileBinding{}
	}
	return c.eventSubsystem.Profile(profile)
}

func (c *Container) MySQLLimiter() backpressure.Acquirer { return c.backpressure.MySQL }

func (c *Container) MongoLimiter() backpressure.Acquirer { return c.backpressure.Mongo }

func (c *Container) PlanEntryBaseURL() string { return c.planEntryURL }

func (c *Container) StatisticsRepairWindowDays() int { return c.statisticsRepairWindowDays }

func (c *Container) ReportStatusConfig() reportstatus.Config { return c.reportStatusConfig }

func (c *Container) StatisticsSystemOptions() statisticsApp.SystemStatisticsOptions {
	opts := c.cacheOptions.StatisticsSystem
	return statisticsApp.SystemStatisticsOptions{
		ServiceSingleflight:     opts.ServiceSingleflight,
		DisableRealtimeFallback: opts.DisableRealtimeFallback,
		StaleOnTimeout:          opts.StaleOnTimeout,
		LoadTimeout:             opts.LoadTimeout,
	}
}

func (c *Container) StatisticsOverviewGuardOptions() statisticsApp.StatisticsReadGuardOptions {
	return toStatisticsReadGuardOptions(c.cacheOptions.StatisticsOverview)
}

func (c *Container) StatisticsQuestionnaireGuardOptions() statisticsApp.StatisticsReadGuardOptions {
	return toStatisticsReadGuardOptions(c.cacheOptions.StatisticsQuestionnaire)
}

func toStatisticsReadGuardOptions(opts cachebootstrap.StatisticsReadGuardOptions) statisticsApp.StatisticsReadGuardOptions {
	return statisticsApp.StatisticsReadGuardOptions{
		ServiceSingleflight: opts.ServiceSingleflight,
		StaleOnTimeout:      opts.StaleOnTimeout,
		LoadTimeout:         opts.LoadTimeout,
	}
}

func (c *Container) CacheObserver() *observability.ComponentObserver {
	return c.cacheObserver()
}

func (c *Container) HotsetRecorder() cachetarget.HotsetRecorder {
	return c.hotsetRecorder()
}

func (c *Container) IdentityService() *iam.IdentityService {
	return c.resolveIdentityService()
}

func (c *Container) ActorIAMPorts() compose.ActorIAMPorts {
	ports := compose.ActorIAMPorts{}
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		ports.Enabled = true
		ports.ProfileLinkService = c.IAMModule.ProfileLinkService()
		ports.IdentityService = c.IAMModule.IdentityService()
		ports.OperationAccountSvc = c.IAMModule.OperationAccountService()
		ports.IAMClient = c.IAMModule.Client()
		ports.AuthzSnapshotLoader = c.IAMModule.AuthzSnapshotLoader()
	}
	return ports
}

func (c *Container) EnsureSurveyRuntimeInfra() (*surveymod.SurveyRuntimeInfra, error) {
	return c.ensureSurveyRuntimeInfra()
}

func (c *Container) SurveyRuntimeInfra() *surveymod.SurveyRuntimeInfra {
	return c.surveyRuntimeInfra
}

func (c *Container) DefaultEvaluationCatalog() (compose.EvaluationCatalog, error) {
	return ammod.ExportEvaluationCatalog()
}

func (c *Container) PublishedModelCatalog() rulesetport.Catalog {
	if c == nil || c.AssessmentModelModule == nil {
		return nil
	}
	return c.AssessmentModelModule.PublishedCatalog
}

func (c *Container) SetWorkbenchLatestRiskReader(reader workbenchreadmodel.LatestRiskReader) {
	c.workbenchLatestRiskReader = reader
}

func (c *Container) SurveyPorts() compose.SurveyPorts {
	ports := compose.SurveyPorts{}
	if c.SurveyModule != nil && c.SurveyModule.Questionnaire != nil {
		ports.QuestionnairePublisher = c.SurveyModule.Questionnaire.LifecycleService
		ports.QuestionnaireQuery = c.SurveyModule.Questionnaire.QueryService
	}
	return ports
}

func (c *Container) ActorPorts() compose.ActorPorts {
	ports := compose.ActorPorts{}
	if c.ActorModule != nil {
		ports.TesteeAccess = c.ActorModule.TesteeAccessService
	}
	return ports
}

func (c *Container) SetSurveyModule(module *surveymod.Module) {
	c.SurveyModule = module
}

func (c *Container) SetAssessmentModelModule(module *ammod.Module) {
	c.AssessmentModelModule = module
	if c.SurveyModule != nil {
		c.SurveyModule.SetCatalogManagementService(module.Management)
	}
	c.registerModule("modelcatalog", module)
}

func (c *Container) SetActorModule(module *actormod.Module) {
	c.ActorModule = module
}

func (c *Container) SetReportModule(module *reportmod.Module) {
	c.ReportModule = module
}

func (c *Container) SetEvaluationModule(module *evalmod.Module) {
	c.EvaluationModule = module
}

func (c *Container) SetPlanModule(module *planmod.Module) {
	c.PlanModule = module
}

func (c *Container) SetStatisticsModule(module *statmod.Module) {
	c.StatisticsModule = module
}

func (c *Container) PlatformState() platformmod.IntegrationState {
	if c == nil {
		return platformmod.IntegrationState{}
	}
	return platformmod.IntegrationState{
		CodesService:                       c.CodesService,
		QRCodeGenerator:                    c.QRCodeGenerator,
		SubscribeSender:                    c.SubscribeSender,
		QRCodeObjectStore:                  c.QRCodeObjectStore,
		QRCodeObjectKeyPrefix:              c.QRCodeObjectKeyPrefix,
		QRCodeService:                      c.QRCodeService,
		MiniProgramTaskNotificationService: c.MiniProgramTaskNotificationService,
	}
}

func (c *Container) ApplyPlatformState(state platformmod.IntegrationState) {
	if c == nil {
		return
	}
	c.CodesService = state.CodesService
	c.QRCodeGenerator = state.QRCodeGenerator
	c.SubscribeSender = state.SubscribeSender
	c.QRCodeObjectStore = state.QRCodeObjectStore
	c.QRCodeObjectKeyPrefix = state.QRCodeObjectKeyPrefix
	c.QRCodeService = state.QRCodeService
	c.MiniProgramTaskNotificationService = state.MiniProgramTaskNotificationService
}

func (c *Container) WeChatAppService() *iam.WeChatAppService {
	if c == nil || c.IAMModule == nil || !c.IAMModule.IsEnabled() {
		return nil
	}
	return c.IAMModule.WeChatAppService()
}

func (c *Container) ProfileLinkService() *iam.ProfileLinkService {
	if c == nil || c.IAMModule == nil || !c.IAMModule.IsEnabled() {
		return nil
	}
	return c.IAMModule.ProfileLinkService()
}

func (c *Container) PublishedModelTitleResolver() modelcatalogApp.PublishedModelTitleResolver {
	lister := c.PublishedModelLister()
	if lister == nil {
		return nil
	}
	return modelcatalogRuntime.NewTitleResolver(lister)
}

func (c *Container) PublishedModelLister() rulesetport.PublishedModelLister {
	if c == nil || c.AssessmentModelModule == nil {
		return nil
	}
	return c.AssessmentModelModule.PublishedLister
}

func (c *Container) TesteeQuery() testeeApp.TesteeQueryService {
	if c == nil || c.ActorModule == nil {
		return nil
	}
	return c.ActorModule.TesteeQueryService
}

func (c *Container) TaskNotificationContext() planApp.TaskNotificationContextReader {
	if c == nil || c.PlanModule == nil {
		return nil
	}
	return c.PlanModule.TaskNotificationContextReader
}

func (c *Container) ensureSurveyRuntimeInfra() (*surveymod.SurveyRuntimeInfra, error) {
	if c == nil {
		return nil, fmt.Errorf("container is nil")
	}
	binding := c.CacheCapability(cachepolicy.CapabilitySurveyQuestionnaire)
	staticRedis := c.CacheClient(redisruntime.FamilyStatic)
	if !binding.Enabled {
		staticRedis = nil
	}
	infra, err := surveymod.EnsureSurveyRuntimeInfraCached(c.surveyRuntimeInfra, surveymod.SurveyRuntimeInfraDeps{
		MongoDB:             c.mongoDB,
		EventCatalog:        c.eventCatalog,
		MongoLimiter:        c.backpressure.Mongo,
		StaticRedis:         staticRedis,
		StaticBuilder:       c.CacheBuilder(redisruntime.FamilyStatic),
		QuestionnairePolicy: binding.Policy,
		Observer:            c.cacheObserver(),
	})
	if err != nil {
		return nil, err
	}
	c.surveyRuntimeInfra = infra
	return infra, nil
}

func (c *Container) resolveIdentityService() *iam.IdentityService {
	if c == nil || c.IAMModule == nil || !c.IAMModule.IsEnabled() {
		return nil
	}
	return c.IAMModule.IdentityService()
}

// compile-time checks
var (
	_ compose.Host            = (*Container)(nil)
	_ surveymod.InstallHost   = (*Container)(nil)
	_ ammod.InstallHost       = (*Container)(nil)
	_ actormod.InstallHost    = (*Container)(nil)
	_ reportmod.InstallHost   = (*Container)(nil)
	_ evalmod.InstallHost     = (*Container)(nil)
	_ planmod.InstallHost     = (*Container)(nil)
	_ statmod.InstallHost     = (*Container)(nil)
	_ platformmod.InstallHost = (*Container)(nil)
)
