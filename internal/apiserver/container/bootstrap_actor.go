package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
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

func (c *Container) buildActorModuleDeps() assembler.ActorModuleDeps {
	deps := c.resolveActorModuleInitDeps()
	return assembler.ActorModuleDeps{
		MySQLDB:             c.mysqlDB,
		GuardianshipService: deps.guardianshipSvc,
		IdentityService:     deps.identitySvc,
		RedisClient:         c.CacheClient(cacheplane.FamilyObject),
		CacheBuilder:        c.CacheBuilder(cacheplane.FamilyObject),
		TesteePolicy:        c.CachePolicy(cachepolicy.PolicyTestee),
		OperatorAuthz:       deps.opAuthz,
		OperationAccountSvc: deps.operationAccountSvc,
		Observer:            c.cacheObserver(),
		TopicResolver:       c.eventCatalog,
		MySQLLimiter:        c.backpressure.MySQL,
	}
}

func (c *Container) buildActorModule() (*assembler.ActorModule, error) {
	return assembler.NewActorModule(c.buildActorModuleDeps())
}

// initActorModule 初始化 Actor 模块。
func (c *Container) initActorModule() error {
	actorModule, err := c.buildActorModule()
	if err != nil {
		return fmt.Errorf("failed to initialize actor module: %w", err)
	}

	c.ActorModule = actorModule
	c.registerModule("actor", actorModule)

	c.printf("📦 Actor module initialized\n")
	return nil
}
