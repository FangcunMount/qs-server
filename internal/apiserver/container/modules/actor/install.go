package actor

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

// InstallHost extends the shared compose seam with actor module bindings.
type InstallHost interface {
	compose.Host
	SetActorModule(*Module)
}

// InstallFrom wires and registers the actor module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	iamPorts := host.ActorIAMPorts()
	provider := host.CachePolicyProvider()
	binding := compose.ResolveCacheCapability(provider, cachepolicy.CapabilityActorTestee)
	redisClient := host.CacheClient(redisruntime.FamilyObject)
	if !binding.Enabled {
		redisClient = nil
	}
	module, err := Wire(WireInput{
		MySQLDB:             host.MySQLDB(),
		RedisClient:         redisClient,
		CacheBuilder:        host.CacheBuilder(redisruntime.FamilyObject),
		CachePolicies:       provider,
		Observer:            host.CacheObserver(),
		MySQLLimiter:        host.MySQLLimiter(),
		IAMEnabled:          iamPorts.Enabled,
		ProfileLinkService:  iamPorts.ProfileLinkService,
		IdentityService:     iamPorts.IdentityService,
		OperationAccountSvc: iamPorts.OperationAccountSvc,
		IAMClient:           iamPorts.IAMClient,
		AuthzSnapshotLoader: iamPorts.AuthzSnapshotLoader,
	})
	if err != nil {
		return err
	}
	host.SetActorModule(module)
	host.RegisterModule("actor", module)
	host.Printf("📦 Actor module initialized\n")
	return nil
}
