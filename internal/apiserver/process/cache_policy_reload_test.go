package process

import (
	"context"
	"testing"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	cachebootstrap "github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/app"
)

type rawSettingsSourceStub struct {
	settings app.RawSettings
}

func (s rawSettingsSourceStub) Read(context.Context) (app.RawSettings, error) { return s.settings, nil }

func TestCachePolicyCandidateLoaderAllowsPolicyAndRejectsEnabled(t *testing.T) {
	startup := options.NewOptions()
	server := &server{config: &config.Config{Options: startup}}
	registry := sharedcache.NewRegistry(cachebootstrap.BuildEffectiveCapabilities(buildContainerCacheOptions(startup.Cache))...)

	startup.SetRawSettingsSource(rawSettingsSourceStub{settings: app.RawSettings{
		Source: "apiserver.yaml",
		Values: cachePolicySettings(map[string]any{"ttl": "9m"}),
	}})
	candidate, source, err := server.cachePolicyCandidateLoader(registry)(context.Background())
	if err != nil {
		t.Fatalf("candidate loader error = %v", err)
	}
	if source != "apiserver.yaml" {
		t.Fatalf("source = %q", source)
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

	startup.SetRawSettingsSource(rawSettingsSourceStub{settings: app.RawSettings{
		Source: "apiserver.yaml",
		Values: cachePolicySettings(map[string]any{"enabled": false}),
	}})
	if _, _, err := server.cachePolicyCandidateLoader(registry)(context.Background()); err == nil || !componenterrors.IsCode(err, code.ErrInvalidArgument) {
		t.Fatalf("enabled change error = %v, want invalid argument", err)
	}
}

func cachePolicySettings(query map[string]any) map[string]any {
	return map[string]any{
		"cache": map[string]any{
			"capabilities": map[string]any{
				"statistics": map[string]any{"query": query},
			},
		},
	}
}
