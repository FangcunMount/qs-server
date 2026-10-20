package plan

import (
	"fmt"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	mysqlEventTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

// InstallHost extends the shared compose seam with plan module bindings.
type InstallHost interface {
	compose.Host
	PublishedModelLister() modelcatalogport.PublishedModelLister
	SetPlanModule(*Module)
	TaskOpenedReminderEnabled() bool
}

// TransactionInstallHost supplies the existing host-owned qs.mysql runtime.
// Sharing it does not transfer lifecycle or database ownership to the module.
type TransactionInstallHost interface {
	PlanTransactionOwner() *mysqlEventTx.Committer
	PlanTransactionRoutes() eventcatalog.TopicResolver
}

// InstallFrom wires and registers the plan module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	provider := host.CachePolicyProvider()
	binding := compose.ResolveCacheCapability(provider, cachepolicy.CapabilityPlanDetail)
	redisClient := host.CacheClient(redisruntime.FamilyObject)
	if !binding.Enabled {
		redisClient = nil
	}
	var owner *mysqlEventTx.Committer
	var routes eventcatalog.TopicResolver
	if transactions, ok := host.(TransactionInstallHost); ok {
		owner = transactions.PlanTransactionOwner()
		routes = transactions.PlanTransactionRoutes()
	}
	reminderEnabled := host.TaskOpenedReminderEnabled()
	var openedOutbox appEventing.ProfileBinding
	if reminderEnabled && owner == nil {
		openedOutbox = host.EventProfile(eventcatalog.OutboxProfileAssessmentMySQL)
		if openedOutbox.Stager == nil {
			return fmt.Errorf("task opened reminder requires a MySQL outbox profile")
		}
	}
	module, err := Wire(WireInput{
		MySQLDB:                   host.MySQLDB(),
		EventPublisher:            host.EventPublisher(),
		PublishedModels:           host.PublishedModelLister(),
		RedisClient:               redisClient,
		CacheBuilder:              host.CacheBuilder(redisruntime.FamilyObject),
		CachePolicies:             provider,
		EntryBaseURL:              host.PlanEntryBaseURL(),
		Observer:                  host.CacheObserver(),
		MySQLLimiter:              host.MySQLLimiter(),
		TesteeAccess:              host.ActorPorts().TesteeAccess,
		OpenedOutbox:              openedOutbox,
		TaskOpenedReminderEnabled: reminderEnabled,
		EventTransactionOwner:     owner,
		TransactionRoutes:         routes,
	})
	if err != nil {
		return err
	}
	host.SetPlanModule(module)
	host.RegisterModule("plan", module)
	host.Printf("📦 Plan module initialized\n")
	return nil
}
