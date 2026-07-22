package process

import (
	"testing"
	"time"

	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
)

func boolPtr(v bool) *bool {
	return &v
}

func TestAPIServerBuildContainerCacheOptions(t *testing.T) {
	opts := apiserveroptions.NewOptions()
	opts.Cache.Capabilities.Survey.Questionnaire.TTL = 3 * time.Minute
	opts.Cache.Capabilities.ModelCatalog.PublishedModel.TTL = time.Minute
	opts.Cache.Capabilities.Evaluation.AssessmentDetail.Enabled = false
	opts.Cache.Capabilities.Evaluation.AssessmentDetail.TTL = 4 * time.Minute
	opts.Cache.Capabilities.Evaluation.AssessmentList.Enabled = false
	opts.Cache.Capabilities.Evaluation.AssessmentList.TTL = 5 * time.Minute
	opts.Cache.Capabilities.Actor.Testee.TTL = 6 * time.Minute
	opts.Cache.Capabilities.Plan.Detail.TTL = 7 * time.Minute
	opts.Cache.Capabilities.Statistics.Query.Enabled = false
	opts.Cache.Capabilities.Statistics.Query.TTL = 30 * time.Second
	opts.Cache.Defaults.TTLJitterRatio = 0.25
	opts.Cache.Defaults.CompressPayload = true
	opts.Cache.Governance.StatisticsWarmup = &apiserveroptions.StatisticsWarmupOptions{
		Enable:          true,
		OrgIDs:          []int64{101, 202},
		OverviewPresets: []string{"latest_complete_day", "30d"},
	}
	opts.Cache.Governance.Warmup = &apiserveroptions.WarmupOptions{
		Enable: true,
		Startup: &apiserveroptions.WarmupStartupOptions{
			Static: true,
			Query:  true,
		},
		Hotset: &apiserveroptions.WarmupHotsetOptions{
			Enable:          true,
			TopN:            50,
			MaxItemsPerKind: 8,
		},
	}
	opts.Cache.Defaults.Static = &apiserveroptions.CacheFamilyOptions{
		NegativeTTL:    9 * time.Second,
		TTLJitterRatio: 0.10,
		Compress:       boolPtr(true),
		Singleflight:   boolPtr(true),
		Negative:       boolPtr(true),
	}
	opts.Cache.Defaults.Object = &apiserveroptions.CacheFamilyOptions{
		NegativeTTL:    10 * time.Second,
		TTLJitterRatio: 0.20,
		Compress:       boolPtr(false),
		Singleflight:   boolPtr(true),
		Negative:       boolPtr(false),
	}
	opts.Cache.Defaults.Query = &apiserveroptions.CacheFamilyOptions{
		NegativeTTL:    6 * time.Second,
		TTLJitterRatio: 0.15,
		Compress:       boolPtr(true),
		Singleflight:   boolPtr(true),
		Negative:       boolPtr(true),
	}
	opts.Signaling.Redis.Enabled = true
	opts.Signaling.Redis.Prefix = "custom:signal"
	opts.Signaling.Redis.Channel = "cache-events"
	opts.Signaling.Redis.BufferSize = 17

	cfg, err := apiserverconfig.CreateConfigFromOptions(opts)
	if err != nil {
		t.Fatalf("CreateConfigFromOptions() error = %v", err)
	}

	server := &server{config: cfg}
	got := server.buildContainerCacheOptions()

	detail := got.Capabilities[cachepolicy.CapabilityEvaluationAssessmentDetail]
	list := got.Capabilities[cachepolicy.CapabilityEvaluationAssessmentList]
	statistics := got.Capabilities[cachepolicy.CapabilityStatisticsQuery]
	if detail.Enabled || list.Enabled || statistics.Enabled {
		t.Fatalf("disabled capability mapping mismatch: detail=%+v list=%+v statistics=%+v", detail, list, statistics)
	}
	if got.Capabilities[cachepolicy.CapabilityModelCatalogPublished].Policy.TTL != time.Minute ||
		got.Capabilities[cachepolicy.CapabilityPlanDetail].Policy.TTL != 7*time.Minute ||
		got.Capabilities[cachepolicy.CapabilityActorTestee].Policy.TTL != 6*time.Minute {
		t.Fatalf("capability TTL mapping mismatch: %+v", got.Capabilities)
	}
	if got.Capabilities[cachepolicy.CapabilityReportStatus].Policy.TTL != 48*time.Hour {
		t.Fatalf("report_status TTL = %v, want 48h", got.Capabilities[cachepolicy.CapabilityReportStatus].Policy.TTL)
	}
	if got.TTLJitterRatio != 0.25 {
		t.Fatalf("TTLJitterRatio = %v, want 0.25", got.TTLJitterRatio)
	}
	if !got.CompressPayload {
		t.Fatalf("CompressPayload = false, want true")
	}
	if got.StatisticsWarmup == nil {
		t.Fatalf("StatisticsWarmup = nil, want value")
	}
	if len(got.StatisticsWarmup.OrgIDs) != 2 || got.StatisticsWarmup.OrgIDs[1] != 202 {
		t.Fatalf("StatisticsWarmup.OrgIDs = %+v", got.StatisticsWarmup.OrgIDs)
	}
	if len(got.StatisticsWarmup.OverviewPresets) != 2 || got.StatisticsWarmup.OverviewPresets[1] != "30d" {
		t.Fatalf("StatisticsWarmup.OverviewPresets = %+v", got.StatisticsWarmup.OverviewPresets)
	}
	if !got.Warmup.Enable || !got.Warmup.StartupStatic || !got.Warmup.StartupQuery {
		t.Fatalf("Warmup mapping mismatch: %+v", got.Warmup)
	}
	if !got.Warmup.HotsetEnable || got.Warmup.HotsetTopN != 50 || got.Warmup.MaxItemsPerKind != 8 {
		t.Fatalf("Hotset mapping mismatch: %+v", got.Warmup)
	}
	if got.Static.NegativeTTL != 9*time.Second || got.Static.Compress == nil || !*got.Static.Compress || got.Static.Singleflight == nil || !*got.Static.Singleflight || got.Static.Negative == nil || !*got.Static.Negative {
		t.Fatalf("Static family mapping mismatch: %+v", got.Static)
	}
	if got.Query.NegativeTTL != 6*time.Second || got.Query.Compress == nil || !*got.Query.Compress {
		t.Fatalf("Query family mapping mismatch: %+v", got.Query)
	}
	if !got.Signal.Enabled || got.Signal.Prefix != "custom:signal" || got.Signal.Channel != "cache-events" || got.Signal.BufferSize != 17 {
		t.Fatalf("Signal options mapping mismatch: %+v", got.Signal)
	}
}

func TestBuildCacheSignalOptionsUsesStableDefaults(t *testing.T) {
	got := buildCacheSignalOptions(nil)
	if got.Enabled || got.Prefix != "qs:signal" || got.Channel != "" || got.BufferSize != 100 {
		t.Fatalf("default signal options = %+v", got)
	}
}

func TestStatisticsRepairWindowDays(t *testing.T) {
	opts := apiserveroptions.NewOptions()
	opts.StatisticsSync = nil

	cfg, err := apiserverconfig.CreateConfigFromOptions(opts)
	if err != nil {
		t.Fatalf("CreateConfigFromOptions() error = %v", err)
	}
	if got := statisticsRepairWindowDays(cfg); got != 0 {
		t.Fatalf("statisticsRepairWindowDays() = %d, want 0", got)
	}

	opts = apiserveroptions.NewOptions()
	opts.StatisticsSync.RepairWindowDays = 14
	cfg, err = apiserverconfig.CreateConfigFromOptions(opts)
	if err != nil {
		t.Fatalf("CreateConfigFromOptions() error = %v", err)
	}
	if got := statisticsRepairWindowDays(cfg); got != 14 {
		t.Fatalf("statisticsRepairWindowDays() = %d, want 14", got)
	}
}

func TestAPIServerBuildContainerOptionsUsesResourceStageCacheSubsystem(t *testing.T) {
	opts := apiserveroptions.NewOptions()
	cfg, err := apiserverconfig.CreateConfigFromOptions(opts)
	if err != nil {
		t.Fatalf("CreateConfigFromOptions() error = %v", err)
	}

	subsystem := &cachebootstrap.Subsystem{}
	server := &server{config: cfg}
	got := server.buildContainerOptions(containerOptionsInput{
		cacheSubsystem: subsystem,
	})

	if got.CacheSubsystem != subsystem {
		t.Fatalf("CacheSubsystem = %#v, want %#v", got.CacheSubsystem, subsystem)
	}
}

func TestBuildEventProfileOptionsMapsOutboxRelayOptions(t *testing.T) {
	opts := apiserveroptions.NewOptions()
	opts.OutboxRelay.Mongo.BatchSize = 360
	opts.OutboxRelay.Mongo.PublishWorkers = 64
	opts.OutboxRelay.Mongo.ImmediateMaxConcurrent = 24
	opts.OutboxRelay.Assessment.BatchSize = 80
	opts.OutboxRelay.Assessment.PublishWorkers = 12
	cfg, err := apiserverconfig.CreateConfigFromOptions(opts)
	if err != nil {
		t.Fatalf("CreateConfigFromOptions() error = %v", err)
	}

	mongoProfile, assessmentProfile := buildEventProfileOptions(cfg)

	if mongoProfile.BatchSize != 360 {
		t.Fatalf("Mongo BatchSize = %d, want 360", mongoProfile.BatchSize)
	}
	if mongoProfile.PublishWorkers != 64 {
		t.Fatalf("Mongo PublishWorkers = %d, want 64", mongoProfile.PublishWorkers)
	}
	if mongoProfile.ImmediateMaxConcurrent != 24 {
		t.Fatalf("Mongo ImmediateMaxConcurrent = %d, want 24", mongoProfile.ImmediateMaxConcurrent)
	}
	if assessmentProfile.BatchSize != 80 {
		t.Fatalf("Assessment BatchSize = %d, want 80", assessmentProfile.BatchSize)
	}
	if assessmentProfile.PublishWorkers != 12 {
		t.Fatalf("Assessment PublishWorkers = %d, want 12", assessmentProfile.PublishWorkers)
	}
}
