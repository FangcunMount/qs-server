package process

import (
	"context"
	"testing"
	"time"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	cachebootstrap "github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type cachePolicySourceStub struct {
	policy *options.CacheOptions
	source sharedcache.PolicySource
}

func (s cachePolicySourceStub) Read(context.Context) (*options.CacheOptions, sharedcache.PolicySource, error) {
	return s.policy, s.source, nil
}

func TestCachePolicyCandidateLoaderAllowsPolicyAndRejectsEnabled(t *testing.T) {
	startup := options.NewOptions()
	server := &server{config: &config.Config{Options: startup}}
	registry := sharedcache.NewRegistry(cachebootstrap.BuildEffectiveCapabilities(buildContainerCacheOptions(startup.Cache, startup.RuntimeState))...)

	candidatePolicy := options.NewCacheOptions()
	candidatePolicy.Capabilities.Statistics.Query.TTL = 9 * time.Minute
	metadata := sharedcache.PolicySource{Component: "qs-apiserver", SchemaVersion: "1.0", Path: "cache/apiserver.yaml", PolicySHA256: "hash-1"}
	startup.SetCachePolicySource(cachePolicySourceStub{policy: candidatePolicy, source: metadata})
	candidate, source, err := server.cachePolicyCandidateLoader(registry)(context.Background())
	if err != nil {
		t.Fatalf("candidate loader error = %v", err)
	}
	if source != metadata {
		t.Fatalf("source = %#v", source)
	}
	var stats sharedcache.EffectiveCapability
	for _, item := range candidate {
		if item.Capability == cachepolicy.CapabilityStatisticsQuery {
			stats = item
		}
	}
	if stats.Policy.TTL.String() != "9m0s" {
		t.Fatalf("statistics.query TTL = %s", stats.Policy.TTL)
	}

	candidatePolicy = options.NewCacheOptions()
	candidatePolicy.Capabilities.Statistics.Query.Enabled = false
	startup.SetCachePolicySource(cachePolicySourceStub{policy: candidatePolicy, source: metadata})
	if _, _, err := server.cachePolicyCandidateLoader(registry)(context.Background()); err == nil || !componenterrors.IsCode(err, code.ErrInvalidArgument) {
		t.Fatalf("enabled change error = %v, want invalid argument", err)
	}
}
