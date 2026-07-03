package process

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
)

func boolPtr(v bool) *bool {
	return &v
}

func TestAPIServerBuildContainerCacheOptions(t *testing.T) {
	opts := apiserveroptions.NewOptions()
	opts.Cache.DisableEvaluationCache = true
	opts.Cache.DisableStatisticsCache = true
	opts.Cache.TTL.Scale = time.Minute
	opts.Cache.TTL.ScaleList = 2 * time.Minute
	opts.Cache.TTL.Questionnaire = 3 * time.Minute
	opts.Cache.TTL.AssessmentDetail = 4 * time.Minute
	opts.Cache.TTL.AssessmentList = 5 * time.Minute
	opts.Cache.TTL.Testee = 6 * time.Minute
	opts.Cache.TTL.Plan = 7 * time.Minute
	opts.Cache.TTL.Negative = 8 * time.Second
	opts.Cache.TTLJitterRatio = 0.25
	opts.Cache.CompressPayload = true
	opts.Cache.StatisticsWarmup = &apiserveroptions.StatisticsWarmupOptions{
		Enable:             true,
		OrgIDs:             []int64{101, 202},
		OverviewPresets:    []string{"today", "30d"},
		QuestionnaireCodes: []string{"phq9", "gad7"},
		PlanIDs:            []uint64{11, 22},
	}
	opts.Cache.Warmup = &apiserveroptions.WarmupOptions{
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
	opts.Cache.Static = &apiserveroptions.CacheFamilyOptions{
		NegativeTTL:    9 * time.Second,
		TTLJitterRatio: 0.10,
		Compress:       boolPtr(true),
		Singleflight:   boolPtr(true),
		Negative:       boolPtr(true),
	}
	opts.Cache.Object = &apiserveroptions.CacheFamilyOptions{
		NegativeTTL:    10 * time.Second,
		TTLJitterRatio: 0.20,
		Compress:       boolPtr(false),
		Singleflight:   boolPtr(true),
		Negative:       boolPtr(false),
	}
	opts.Cache.Query = &apiserveroptions.CacheFamilyOptions{
		TTL:            30 * time.Second,
		NegativeTTL:    6 * time.Second,
		TTLJitterRatio: 0.15,
		Compress:       boolPtr(true),
		Singleflight:   boolPtr(true),
		Negative:       boolPtr(true),
	}
	opts.Cache.SDK = &apiserveroptions.CacheFamilyOptions{
		NegativeTTL:    5 * time.Second,
		TTLJitterRatio: 0.05,
		Compress:       boolPtr(false),
		Singleflight:   boolPtr(true),
		Negative:       boolPtr(true),
	}
	opts.Cache.Lock = &apiserveroptions.CacheFamilyOptions{
		NegativeTTL:    3 * time.Second,
		TTLJitterRatio: 0.01,
		Compress:       boolPtr(false),
		Singleflight:   boolPtr(false),
		Negative:       boolPtr(false),
	}

	cfg, err := apiserverconfig.CreateConfigFromOptions(opts)
	if err != nil {
		t.Fatalf("CreateConfigFromOptions() error = %v", err)
	}

	server := &server{config: cfg}
	got := server.buildContainerCacheOptions()

	if !got.DisableEvaluationCache {
		t.Fatalf("DisableEvaluationCache = false, want true")
	}
	if !got.DisableStatisticsCache {
		t.Fatalf("DisableStatisticsCache = false, want true")
	}
	if got.TTL.Scale != time.Minute || got.TTL.Plan != 7*time.Minute || got.TTL.Negative != 8*time.Second {
		t.Fatalf("TTL mapping mismatch: %+v", got.TTL)
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
	if got.Query.TTL != 30*time.Second || got.Query.NegativeTTL != 6*time.Second || got.Query.Compress == nil || !*got.Query.Compress {
		t.Fatalf("Query family mapping mismatch: %+v", got.Query)
	}
	if got.SDK.NegativeTTL != 5*time.Second || got.SDK.Singleflight == nil || !*got.SDK.Singleflight {
		t.Fatalf("SDK family mapping mismatch: %+v", got.SDK)
	}
	if got.Lock.NegativeTTL != 3*time.Second || got.Lock.Singleflight == nil || *got.Lock.Singleflight {
		t.Fatalf("Lock family mapping mismatch: %+v", got.Lock)
	}
	if got.Meta != (container.ContainerCacheFamilyOptions{}) {
		t.Fatalf("Meta family = %+v, want zero value", got.Meta)
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

func TestBuildStatisticsOverviewOptionsMapsServiceSingleflight(t *testing.T) {
	opts := apiserveroptions.NewOptions()
	opts.Cache.StatisticsOverview = &apiserveroptions.StatisticsOverviewOptions{
		ServiceSingleflight: false,
		StaleOnTimeout:      true,
		LoadTimeout:         11 * time.Second,
	}

	got := buildStatisticsOverviewOptions(opts.Cache)
	if got.ServiceSingleflight {
		t.Fatal("ServiceSingleflight = true, want false")
	}
	if !got.StaleOnTimeout || got.LoadTimeout != 11*time.Second {
		t.Fatalf("guard options = %+v", got)
	}
}

func TestBuildStatisticsOverviewOptionsDefaultsServiceSingleflightTrue(t *testing.T) {
	got := buildStatisticsOverviewOptions(apiserveroptions.NewOptions().Cache)
	if !got.ServiceSingleflight {
		t.Fatal("ServiceSingleflight = false, want true")
	}
}

func TestBuildStatisticsQuestionnaireOptionsMapsGuardFields(t *testing.T) {
	opts := apiserveroptions.NewOptions()
	opts.Cache.StatisticsQuestionnaire = &apiserveroptions.StatisticsQuestionnaireOptions{
		ServiceSingleflight: false,
		StaleOnTimeout:      false,
		LoadTimeout:         9 * time.Second,
	}

	got := buildStatisticsQuestionnaireOptions(opts.Cache)
	if got.ServiceSingleflight {
		t.Fatal("ServiceSingleflight = true, want false")
	}
	if got.StaleOnTimeout {
		t.Fatal("StaleOnTimeout = true, want false")
	}
	if got.LoadTimeout != 9*time.Second {
		t.Fatalf("LoadTimeout = %v, want 9s", got.LoadTimeout)
	}
}

func TestBuildStatisticsQuestionnaireOptionsDefaultsServiceSingleflightTrue(t *testing.T) {
	got := buildStatisticsQuestionnaireOptions(apiserveroptions.NewOptions().Cache)
	if !got.ServiceSingleflight {
		t.Fatal("ServiceSingleflight = false, want true")
	}
}

func TestAPIServerBuildContainerOptionsMapsOutboxRelayOptions(t *testing.T) {
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

	server := &server{config: cfg}
	got := server.buildContainerOptions(containerOptionsInput{})

	if got.OutboxRelay.MongoBatchSize != 360 {
		t.Fatalf("MongoBatchSize = %d, want 360", got.OutboxRelay.MongoBatchSize)
	}
	if got.OutboxRelay.MongoPublishWorkers != 64 {
		t.Fatalf("MongoPublishWorkers = %d, want 64", got.OutboxRelay.MongoPublishWorkers)
	}
	if got.OutboxRelay.MongoImmediateMaxConcurrent != 24 {
		t.Fatalf("MongoImmediateMaxConcurrent = %d, want 24", got.OutboxRelay.MongoImmediateMaxConcurrent)
	}
	if got.OutboxRelay.AssessmentBatchSize != 80 {
		t.Fatalf("AssessmentBatchSize = %d, want 80", got.OutboxRelay.AssessmentBatchSize)
	}
	if got.OutboxRelay.AssessmentPublishWorkers != 12 {
		t.Fatalf("AssessmentPublishWorkers = %d, want 12", got.OutboxRelay.AssessmentPublishWorkers)
	}
}
