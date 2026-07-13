package cache

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
)

func TestSubsystemBuildsConfiguredTypedCaches(t *testing.T) {
	opts := options.NewOptions()
	opts.Cache.Capabilities.Catalog.Questionnaire.Enabled = true
	opts.Cache.Capabilities.Catalog.Questionnaire.Singleflight = false
	opts.Cache.Capabilities.Catalog.Typology.Enabled = true
	opts.Cache.Capabilities.Catalog.Typology.Singleflight = true

	s := NewSubsystem(testConfig(opts), nil)
	if s.Questionnaire() == nil {
		t.Fatal("questionnaire cache = nil, want configured cache")
	}
	if s.Typology() == nil {
		t.Fatal("typology cache = nil, want configured cache")
	}
	if s.QuestionnaireSingleflight() {
		t.Fatal("questionnaire singleflight = true, want false")
	}
	if !s.TypologySingleflight() {
		t.Fatal("typology singleflight = false, want true")
	}
	entries := s.EffectiveRegistry().All()
	if len(entries) != 3 || entries[0].Capability != "catalog.questionnaire" || entries[1].Capability != "catalog.typology" || entries[2].Kind != "operational_state" {
		t.Fatalf("effective registry = %#v", entries)
	}
}

func TestSubsystemDisabledCachesStayNil(t *testing.T) {
	opts := options.NewOptions()
	opts.Cache.Capabilities.Catalog.Questionnaire.Enabled = false
	opts.Cache.Capabilities.Catalog.Typology.Enabled = false

	s := NewSubsystem(testConfig(opts), nil)
	if s.Questionnaire() != nil || s.Typology() != nil {
		t.Fatal("disabled cache was constructed")
	}
}

func TestSubsystemStartCloseAreIdempotent(t *testing.T) {
	s := NewSubsystem(testConfig(options.NewOptions()), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	firstCancel := s.cancel
	if firstCancel == nil {
		t.Fatal("Start() did not install lifecycle cancel")
	}
	if err := s.Start(ctx); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if s.cancel == nil {
		t.Fatal("second Start() cleared lifecycle cancel")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if s.started || s.cancel != nil {
		t.Fatalf("closed lifecycle state = started:%v cancel:%v", s.started, s.cancel != nil)
	}
}

func TestSignalEvictRequiresEnabledCache(t *testing.T) {
	if (CatalogBinding{Enabled: false, SignalEvict: true}).Enabled {
		t.Fatal("signal eviction enabled for disabled cache")
	}
	if binding := (CatalogBinding{Enabled: true, SignalEvict: true}); !binding.Enabled || !binding.SignalEvict {
		t.Fatal("signal eviction disabled for enabled cache")
	}
}

func testConfig(opts *options.Options) Config {
	config := Config{Signaling: cachesignal.ConfigFromOptions(opts.Signaling, "collection-server")}
	catalog := opts.Cache.Capabilities.Catalog
	config.Questionnaire = testBinding("catalog.questionnaire", &catalog.Questionnaire.CatalogL1CacheOptions)
	config.Typology = testBinding("catalog.typology", &catalog.Typology.CatalogL1CacheOptions)
	config.ReportStatusTTL = time.Duration(opts.Cache.Capabilities.ReportStatus.TTLSeconds) * time.Second
	return config
}

func testBinding(id string, cfg *options.CatalogL1CacheOptions) CatalogBinding {
	return CatalogBinding{
		Capability: sharedcache.Capability(id), Source: "cache.capabilities." + id,
		Enabled: cfg.Enabled, Policy: sharedcache.Policy{
			TTL: time.Duration(cfg.TTLSeconds) * time.Second, JitterRatio: cfg.TTLJitterRatio,
			Singleflight: sharedcache.PolicySwitchFromBool(cfg.Singleflight),
		},
		MaxEntries: cfg.MaxEntries, Singleflight: cfg.Singleflight, SignalEvict: cfg.SignalEvictEnabled,
	}
}
