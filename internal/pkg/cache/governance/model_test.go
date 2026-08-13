package governance

import (
	"testing"
	"time"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
)

func TestProjectRegistryPreservesComponentNeutralPolicyContract(t *testing.T) {
	registry := sharedcache.NewRegistryWithSource(sharedcache.PolicySource{
		Component: "collection-server", SchemaVersion: "1.0", Path: "/cache/collection.prod.yaml", PolicySHA256: "abc",
	}, sharedcache.EffectiveCapability{
		Capability: "catalog.questionnaire", Owner: "catalog", Kind: sharedcache.KindCache,
		Layer: sharedcache.LayerL1, Enabled: true, CatalogVersion: "v2", MetricLabel: "questionnaire",
		Policy: sharedcache.Policy{TTL: time.Minute, Singleflight: sharedcache.PolicySwitchEnabled},
	})

	got := ProjectRegistry(registry, PolicyReloadStatus{})
	if got.SnapshotVersion != 1 || got.CatalogVersion != "v2" {
		t.Fatalf("registry identity = %#v", got)
	}
	if got.PolicySource == nil || got.PolicySource.Component != "collection-server" || got.PolicySource.PolicySHA256 != "abc" {
		t.Fatalf("policy source = %#v", got.PolicySource)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0].Layer != "L1" || got.Capabilities[0].Effective.TTL != "1m0s" {
		t.Fatalf("capabilities = %#v", got.Capabilities)
	}
}
