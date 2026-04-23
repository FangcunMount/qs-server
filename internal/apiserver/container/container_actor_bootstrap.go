package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

type actorModuleInitDeps struct {
	guardianshipSvc     *iam.GuardianshipService
	identitySvc         *iam.IdentityService
	operationAccountSvc *iam.OperationAccountService
	opAuthz             *iam.OperatorAuthzBundle
}

func (c *Container) resolveActorModuleInitDeps() actorModuleInitDeps {
	deps := actorModuleInitDeps{}
	if c == nil || c.IAMModule == nil || !c.IAMModule.IsEnabled() {
		return deps
	}

	deps.guardianshipSvc = c.IAMModule.GuardianshipService()
	deps.identitySvc = c.IAMModule.IdentityService()
	deps.operationAccountSvc = c.IAMModule.OperationAccountService()
	deps.opAuthz = &iam.OperatorAuthzBundle{
		Assignment: iam.NewAuthzAssignmentClient(c.IAMModule.Client()),
		Snapshot:   c.IAMModule.AuthzSnapshotLoader(),
	}
	return deps
}

func (c *Container) buildActorModuleInitializeParams() []interface{} {
	deps := c.resolveActorModuleInitDeps()
	return []interface{}{
		c.mysqlDB,
		deps.guardianshipSvc,
		deps.identitySvc,
		c.CacheClient(redisplane.FamilyObject),
		c.CacheBuilder(redisplane.FamilyObject),
		c.CachePolicy(cachepolicy.PolicyTestee),
		deps.opAuthz,
		deps.operationAccountSvc,
		c.cacheObserver(),
	}
}

// initActorModule 初始化 Actor 模块。
func (c *Container) initActorModule() error {
	actorModule := assembler.NewActorModule()
	if err := actorModule.Initialize(c.buildActorModuleInitializeParams()...); err != nil {
		return fmt.Errorf("failed to initialize actor module: %w", err)
	}

	c.ActorModule = actorModule
	c.registerModule("actor", actorModule)

	c.printf("📦 Actor module initialized\n")
	return nil
}

func (c *Container) wireActorEvaluationDependencies() {
	if c == nil || c.ActorModule == nil || c.EvaluationModule == nil {
		return
	}

	c.ActorModule.SetEvaluationServices(
		c.EvaluationModule.ManagementService,
		c.EvaluationModule.ScoreQueryService,
	)
}

func (c *Container) wireProtectedScopeDependencies() {
	if c == nil || c.ActorModule == nil || c.ActorModule.TesteeAccessService == nil {
		return
	}

	if c.EvaluationModule != nil {
		c.EvaluationModule.SetTesteeAccessService(c.ActorModule.TesteeAccessService)
	}
	if c.PlanModule != nil {
		c.PlanModule.SetTesteeAccessService(c.ActorModule.TesteeAccessService)
	}
	if c.StatisticsModule != nil {
		c.StatisticsModule.SetTesteeAccessService(c.ActorModule.TesteeAccessService)
	}
}
