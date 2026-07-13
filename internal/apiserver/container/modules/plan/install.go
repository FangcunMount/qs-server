package plan

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

// InstallHost extends the shared compose seam with plan module bindings.
type InstallHost interface {
	compose.Host
	PublishedModelLister() modelcatalogport.PublishedModelLister
	SetPlanModule(*Module)
}

// InstallFrom wires and registers the plan module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	binding := host.CacheCapability(cachepolicy.CapabilityPlanDetail)
	redisClient := host.CacheClient(redisruntime.FamilyObject)
	if !binding.Enabled {
		redisClient = nil
	}
	module, err := Wire(WireInput{
		MySQLDB:         host.MySQLDB(),
		EventPublisher:  host.EventPublisher(),
		PublishedModels: host.PublishedModelLister(),
		RedisClient:     redisClient,
		CacheBuilder:    host.CacheBuilder(redisruntime.FamilyObject),
		PlanPolicy:      binding.Policy,
		EntryBaseURL:    host.PlanEntryBaseURL(),
		Observer:        host.CacheObserver(),
		MySQLLimiter:    host.MySQLLimiter(),
		TesteeAccess:    host.ActorPorts().TesteeAccess,
	})
	if err != nil {
		return err
	}
	host.SetPlanModule(module)
	host.RegisterModule("plan", module)
	host.Printf("📦 Plan module initialized\n")
	return nil
}
