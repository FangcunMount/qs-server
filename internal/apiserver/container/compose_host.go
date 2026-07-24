package container

import (
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	modelcatalogApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	modelcatalogRuntime "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/runtime"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	cachetarget "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
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
	domainreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/workbenchreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
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

func (c *Container) MySQLLimiter() backpressure.Acquirer {
	if c == nil || c.resilience == nil {
		return nil
	}
	return c.resilience.Backpressure("mysql")
}

func (c *Container) MongoLimiter() backpressure.Acquirer {
	if c == nil || c.resilience == nil {
		return nil
	}
	return c.resilience.Backpressure("mongo")
}

func (c *Container) PlanEntryBaseURL() string { return c.planEntryURL }

func (c *Container) StatisticsRepairWindowDays() int { return c.statisticsRepairWindowDays }

func (c *Container) ReportStatusConfig() reportstatus.Config { return c.reportStatusConfig }

func (c *Container) InterpretationRunLeaseDuration() time.Duration {
	defaults := apiserveroptions.NewInterpretationLeaseGovernanceOptions()
	if c == nil || c.systemGovernanceOptions == nil || c.systemGovernanceOptions.Retry == nil || c.systemGovernanceOptions.Retry.Lease == nil {
		return defaults.RunLeaseDuration()
	}
	return c.systemGovernanceOptions.Retry.Lease.RunLeaseDuration()
}

func (c *Container) PublishedReportTemplateCatalog() domainreporttemplate.Catalog {
	if c == nil || c.ReportModule == nil {
		return nil
	}
	return c.ReportModule.ReportTemplateCatalog()
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
		ports.TesteeQuery = c.ActorModule.TesteeQueryService
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
		if module.PublishedCatalog != nil {
			c.SurveyModule.SetAssessmentBindingResolver(rulesetInfra.NewAssessmentBindingResolver(module.PublishedCatalog))
		}
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

func (c *Container) PublishedModelWarmer() cachetarget.PublishedModelWarmer {
	if c == nil || c.AssessmentModelModule == nil {
		return nil
	}
	return c.AssessmentModelModule.PublishedWarmer
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
	provider := c.CachePolicyProvider()
	binding := compose.ResolveCacheCapability(provider, cachepolicy.CapabilitySurveyQuestionnaire)
	staticRedis := c.CacheClient(redisruntime.FamilyStatic)
	if !binding.Enabled {
		staticRedis = nil
	}
	infra, err := surveymod.EnsureSurveyRuntimeInfraCached(c.surveyRuntimeInfra, surveymod.SurveyRuntimeInfraDeps{
		MongoDB:       c.mongoDB,
		MongoLimiter:  c.MongoLimiter(),
		StaticRedis:   staticRedis,
		StaticBuilder: c.CacheBuilder(redisruntime.FamilyStatic),
		CachePolicies: provider,
		Observer:      c.cacheObserver(),
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
