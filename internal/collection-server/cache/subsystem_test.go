package cache

import (
	"context"
	"testing"
	"time"

	appmodelcatalog "github.com/FangcunMount/qs-server/internal/collection-server/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	cachesignal "github.com/FangcunMount/qs-server/internal/pkg/cache/signal"
)

func TestSubsystemBuildsConfiguredTypedCaches(t *testing.T) {
	opts := options.NewOptions()
	opts.Cache.Capabilities.Catalog.Questionnaire.Enabled = true
	opts.Cache.Capabilities.Catalog.Questionnaire.Singleflight = false
	opts.Cache.Capabilities.Catalog.PublishedModel.Enabled = true
	opts.Cache.Capabilities.Catalog.PublishedModel.Singleflight = true
	opts.Cache.Capabilities.Catalog.Typology.Enabled = true
	opts.Cache.Capabilities.Catalog.Typology.Singleflight = true
	opts.Cache.Capabilities.Evaluation.AssessmentDetail.Enabled = true
	opts.Cache.Capabilities.Evaluation.AssessmentDetail.Singleflight = true
	opts.Cache.Capabilities.Evaluation.AssessmentAccess.Enabled = true
	opts.Cache.Capabilities.Evaluation.AssessmentAccess.Singleflight = true

	s := NewSubsystem(testConfig(opts), nil)
	if s.Questionnaire() == nil {
		t.Fatal("questionnaire cache = nil, want configured cache")
	}
	if s.Typology() == nil {
		t.Fatal("typology cache = nil, want configured cache")
	}
	if s.PublishedModel() == nil {
		t.Fatal("published model cache = nil, want configured cache")
	}
	if s.AssessmentDetail() == nil {
		t.Fatal("assessment detail cache = nil, want configured cache")
	}
	if s.AssessmentAccess() == nil {
		t.Fatal("assessment access cache = nil, want configured cache")
	}
	if s.QuestionnaireSingleflight() {
		t.Fatal("questionnaire singleflight = true, want false")
	}
	if !s.TypologySingleflight() {
		t.Fatal("typology singleflight = false, want true")
	}
	if !s.PublishedModelSingleflight() {
		t.Fatal("published model singleflight = false, want true")
	}
	if !s.AssessmentDetailSingleflight() {
		t.Fatal("assessment detail singleflight = false, want true")
	}
	if !s.AssessmentAccessSingleflight() {
		t.Fatal("assessment access singleflight = false, want true")
	}
	entries := s.EffectiveRegistry().All()
	if len(entries) != 6 || entries[0].Capability != "catalog.published_model" || entries[1].Capability != "catalog.questionnaire" || entries[2].Capability != "catalog.typology" || entries[3].Capability != "evaluation.assessment_access" || entries[4].Capability != "evaluation.assessment_detail" || entries[5].Kind != "operational_state" {
		t.Fatalf("effective registry = %#v", entries)
	}
	if entries[0].CatalogVersion != "v3" || entries[0].TopologyGroup != "published-model" || entries[0].TopologyOrder != 10 {
		t.Fatalf("published-model topology = %#v", entries[0])
	}
	if entries[2].TopologyGroup != "" {
		t.Fatalf("typology must remain outside fixed topology: %#v", entries[2])
	}
}

func TestSubsystemDisabledCachesStayNil(t *testing.T) {
	opts := options.NewOptions()
	opts.Cache.Capabilities.Catalog.Questionnaire.Enabled = false
	opts.Cache.Capabilities.Catalog.PublishedModel.Enabled = false
	opts.Cache.Capabilities.Catalog.Typology.Enabled = false
	opts.Cache.Capabilities.Evaluation.AssessmentDetail.Enabled = false
	opts.Cache.Capabilities.Evaluation.AssessmentAccess.Enabled = false

	s := NewSubsystem(testConfig(opts), nil)
	if s.Questionnaire() != nil || s.PublishedModel() != nil || s.Typology() != nil || s.AssessmentDetail() != nil || s.AssessmentAccess() != nil {
		t.Fatal("disabled cache was constructed")
	}
	published, ok := s.EffectiveRegistry().Resolve("catalog.published_model")
	if !ok || published.Enabled || published.Layer != sharedcache.LayerL1 {
		t.Fatalf("disabled published-model registry entry = %#v, found=%v", published, ok)
	}
	runtime := s.L1Runtime()
	if len(runtime) != 5 {
		t.Fatalf("disabled L1 runtime rows = %#v, want all five capabilities", runtime)
	}
	for _, row := range runtime {
		if row.Enabled || len(row.Buckets) != 0 || row.SignalWatcher.Status != "disabled_by_policy" {
			t.Fatalf("disabled L1 runtime row = %#v", row)
		}
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

func TestPublishedModelSignalEvictsExactDetailAndAllDerivedBuckets(t *testing.T) {
	opts := options.NewOptions()
	opts.Cache.Capabilities.Catalog.PublishedModel.Enabled = true
	s := NewSubsystem(testConfig(opts), nil)
	cache := s.PublishedModel()
	cache.SetDetail("scale-a", &appmodelcatalog.ModelResponse{ModelSummaryResponse: appmodelcatalog.ModelSummaryResponse{Code: "scale-a"}})
	cache.SetDetail("scale-b", &appmodelcatalog.ModelResponse{ModelSummaryResponse: appmodelcatalog.ModelSummaryResponse{Code: "scale-b"}})
	cache.SetListByRequest(&appmodelcatalog.ListRequest{}, &appmodelcatalog.ListResponse{})
	cache.SetOptions("", &appmodelcatalog.OptionsResponse{})

	s.evictPublishedModelOnSignal(cachesignal.AssessmentModelCacheChangedSignal{Kind: "scale", Code: "SCALE-A", Action: "publish"})
	if _, ok := cache.GetDetail("scale-a"); ok {
		t.Fatal("changed detail remained cached")
	}
	if _, ok := cache.GetDetail("scale-b"); !ok {
		t.Fatal("unrelated detail was evicted")
	}
	if _, ok := cache.GetListByRequest(&appmodelcatalog.ListRequest{}); ok {
		t.Fatal("published-model list remained cached")
	}
	if _, ok := cache.GetOptions(""); ok {
		t.Fatal("published-model options remained cached")
	}
	for _, row := range s.L1Runtime() {
		if row.Capability != "catalog.published_model" {
			continue
		}
		if len(row.Buckets) != 3 {
			t.Fatalf("published-model buckets = %#v", row.Buckets)
		}
		for _, bucket := range row.Buckets {
			if bucket.SignalDeletions != 1 {
				t.Fatalf("bucket %q signal deletions = %d, want 1", bucket.Bucket, bucket.SignalDeletions)
			}
		}
		return
	}
	t.Fatal("published-model L1 runtime row not found")
}

func TestSignalOptionsRedisOptions(t *testing.T) {
	defaults := (SignalOptions{}).redisOptions()
	if defaults.Prefix != "qs:signal" || defaults.BufferSize != 100 || defaults.Channel != "" {
		t.Fatalf("default Redis options = %+v", defaults)
	}
	overrides := (SignalOptions{Prefix: "custom", Channel: "cache-events", BufferSize: 9}).redisOptions()
	if overrides.Prefix != "custom" || overrides.Channel != "cache-events" || overrides.BufferSize != 9 {
		t.Fatalf("overridden Redis options = %+v", overrides)
	}
}

func testConfig(opts *options.Options) Config {
	config := Config{Signaling: SignalOptions{Prefix: "qs:signal", BufferSize: 100}}
	if opts.Signaling != nil && opts.Signaling.Redis != nil {
		redis := opts.Signaling.Redis
		config.Signaling.Enabled = redis.Enabled
		if redis.Prefix != "" {
			config.Signaling.Prefix = redis.Prefix
		}
		config.Signaling.Channel = redis.Channel
		if redis.BufferSize > 0 {
			config.Signaling.BufferSize = redis.BufferSize
		}
	}
	catalog := opts.Cache.Capabilities.Catalog
	config.Questionnaire = testBinding("catalog.questionnaire", &catalog.Questionnaire.CatalogL1CacheOptions)
	config.PublishedModel = testBinding("catalog.published_model", &catalog.PublishedModel.CatalogL1CacheOptions)
	config.Typology = testBinding("catalog.typology", &catalog.Typology.CatalogL1CacheOptions)
	evaluation := opts.Cache.Capabilities.Evaluation
	config.AssessmentDetail = testBinding("evaluation.assessment_detail", &evaluation.AssessmentDetail.CatalogL1CacheOptions)
	config.AssessmentAccess = testBinding("evaluation.assessment_access", &evaluation.AssessmentAccess.CatalogL1CacheOptions)
	config.ReportStatusTTL = opts.RuntimeState.ReportStatus.TTL()
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
