package configcontract

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	collectionconfig "github.com/FangcunMount/qs-server/internal/collection-server/config"
	collectionoptions "github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
	workeroptions "github.com/FangcunMount/qs-server/internal/worker/options"
	"github.com/spf13/viper"
)

func TestAPIServerDevProdConfigContracts(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"apiserver.dev.yaml", "apiserver.prod.yaml"} {
		t.Run(name, func(t *testing.T) {
			opts := apiserveroptions.NewOptions()
			loadConfig(t, filepath.Join(repoRoot(t), "configs", name), opts)
			stubSecureTLSFiles(t, opts.SecureServing)
			completeAndValidate(t, opts)
			cfg, err := apiserverconfig.CreateConfigFromOptions(opts)
			if err != nil {
				t.Fatalf("CreateConfigFromOptions() error = %v", err)
			}
			if cfg.Options != opts {
				t.Fatal("config must wrap the decoded apiserver options")
			}
			assertRedisFamilies(t, opts.RedisRuntime, []redisruntime.Family{
				redisruntime.FamilyStatic,
				redisruntime.FamilyObject,
				redisruntime.FamilyQuery,
				redisruntime.FamilyMeta,
				redisruntime.FamilyRank,
				redisruntime.FamilySDK,
				redisruntime.FamilyLock,
			})
			if opts.MessagingOptions == nil {
				t.Fatal("messaging options must be traceable")
			}
			if opts.Backpressure == nil || opts.Backpressure.MySQL == nil || opts.Backpressure.Mongo == nil || opts.Backpressure.IAM == nil {
				t.Fatal("apiserver backpressure config must include mysql, mongo, and iam")
			}
			if opts.IAMOptions == nil || opts.IAMOptions.ServiceAuth == nil {
				t.Fatal("apiserver IAM service auth config must be traceable")
			}
			assertSystemGovernanceConfig(t, name, opts.SystemGovernance)
			assertIAMJWKSURLContract(t, "apiserver", name, opts.IAMOptions)
			assertEventCatalogLoads(t)
		})
	}
}

func TestCollectionDevProdConfigContracts(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"collection-server.dev.yaml", "collection-server.prod.yaml"} {
		t.Run(name, func(t *testing.T) {
			opts := collectionoptions.NewOptions()
			loadConfig(t, filepath.Join(repoRoot(t), "configs", name), opts)
			stubSecureTLSFiles(t, opts.SecureServing)
			completeAndValidate(t, opts)
			cfg, err := collectionconfig.CreateConfigFromOptions(opts)
			if err != nil {
				t.Fatalf("CreateConfigFromOptions() error = %v", err)
			}
			if cfg.Options != opts {
				t.Fatal("config must wrap the decoded collection options")
			}
			assertRedisFamilies(t, opts.RedisRuntime, []redisruntime.Family{
				redisruntime.FamilyOps,
				redisruntime.FamilyLock,
			})
			if opts.RateLimit == nil || !opts.RateLimit.Enabled {
				t.Fatal("collection rate limit config must be traceable and enabled by default")
			}
			if opts.SubmitQueue == nil || opts.SubmitQueue.QueueSize <= 0 || opts.SubmitQueue.WorkerCount <= 0 {
				t.Fatal("collection submit queue config must define positive queue size and worker count")
			}
			if opts.IAMOptions == nil || opts.IAMOptions.ServiceAuth == nil {
				t.Fatal("collection IAM service auth config must be traceable")
			}
			assertIAMJWKSURLContract(t, "collection", name, opts.IAMOptions)
		})
	}
}

func TestWorkerDevProdConfigContracts(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"worker.dev.yaml", "worker.prod.yaml"} {
		t.Run(name, func(t *testing.T) {
			opts := workeroptions.NewOptions()
			loadConfig(t, filepath.Join(repoRoot(t), "configs", name), opts)
			completeAndValidate(t, opts)
			cfg, err := workerconfig.CreateConfigFromOptions(opts)
			if err != nil {
				t.Fatalf("CreateConfigFromOptions() error = %v", err)
			}
			if cfg.Options != opts {
				t.Fatal("config must wrap the decoded worker options")
			}
			assertRedisFamilies(t, opts.RedisRuntime, []redisruntime.Family{
				redisruntime.FamilyLock,
			})
			if cfg.Messaging == nil || cfg.Messaging.Provider == "" {
				t.Fatal("worker messaging config must be traceable")
			}
			if cfg.Worker == nil || cfg.Worker.Concurrency <= 0 {
				t.Fatal("worker runtime config must define positive concurrency")
			}
			if workerEventConfigPath(cfg.Worker) != "configs/events.yaml" {
				t.Fatalf("worker event config fallback = %q, want configs/events.yaml", workerEventConfigPath(cfg.Worker))
			}
			assertEventCatalogLoads(t)
		})
	}
}

func TestReportStatusTTLContractMatchesAcrossProcesses(t *testing.T) {
	t.Parallel()

	for _, suffix := range []string{"dev", "prod"} {
		api := apiserveroptions.NewOptions()
		loadConfig(t, filepath.Join(repoRoot(t), "configs", "apiserver."+suffix+".yaml"), api)
		collection := collectionoptions.NewOptions()
		loadConfig(t, filepath.Join(repoRoot(t), "configs", "collection-server."+suffix+".yaml"), collection)
		worker := workeroptions.NewOptions()
		loadConfig(t, filepath.Join(repoRoot(t), "configs", "worker."+suffix+".yaml"), worker)

		want := api.Cache.Capabilities.ReportStatus.TTLSeconds
		collectionTTL := collection.Cache.Capabilities.ReportStatus.TTLSeconds
		workerTTL := worker.Cache.Capabilities.ReportStatus.TTLSeconds
		if collectionTTL != want || workerTTL != want {
			t.Fatalf("%s report status TTL mismatch: api=%d collection=%d worker=%d", suffix, want, collectionTTL, workerTTL)
		}
	}
}

func loadConfig(t *testing.T, path string, target any) {
	t.Helper()
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig(%s) error = %v", path, err)
	}
	if validator, ok := target.(interface{ ValidateRawSettings(map[string]any) error }); ok {
		if err := validator.ValidateRawSettings(v.AllSettings()); err != nil {
			t.Fatalf("ValidateRawSettings(%s) error = %v", path, err)
		}
	}
	if err := v.Unmarshal(target); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", path, err)
	}
}

func assertIAMJWKSURLContract(t *testing.T, service, configName string, opts *genericoptions.IAMOptions) {
	t.Helper()
	if opts == nil || opts.JWKS == nil {
		t.Fatalf("%s %s IAM JWKS config must be traceable", service, configName)
	}
	rawURL := strings.TrimSpace(opts.JWKS.URL)
	if rawURL == "" {
		t.Fatalf("%s %s iam.jwks.url must not be empty", service, configName)
	}
	if strings.Contains(rawURL, "/api/v1/.well-known") {
		t.Fatalf("%s %s iam.jwks.url must not use retired IAM v1 JWKS path: %s", service, configName, rawURL)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("%s %s iam.jwks.url must be parseable: %v", service, configName, err)
	}
	switch parsed.Path {
	case "/.well-known/jwks.json", "/api/v2/.well-known/jwks.json":
	default:
		t.Fatalf("%s %s iam.jwks.url path = %q, want /.well-known/jwks.json or /api/v2/.well-known/jwks.json", service, configName, parsed.Path)
	}
}

func assertSystemGovernanceConfig(t *testing.T, configName string, opts *apiserveroptions.SystemGovernanceOptions) {
	t.Helper()
	if opts == nil {
		t.Fatalf("%s system_governance config must be traceable", configName)
	}
	if opts.Prometheus == nil {
		t.Fatalf("%s system_governance.prometheus must be present", configName)
	}
	if strings.TrimSpace(opts.Prometheus.BaseURL) == "" {
		t.Fatalf("%s system_governance.prometheus.base_url must not be empty", configName)
	}
	if opts.Prometheus.Timeout <= 0 {
		t.Fatalf("%s system_governance.prometheus.timeout must be positive", configName)
	}
	if len(opts.Components) == 0 {
		t.Fatalf("%s system_governance.components must configure remote governance components", configName)
	}
	for _, component := range []string{"collection-server", "worker"} {
		cfg := opts.Components[component]
		if cfg == nil {
			t.Fatalf("%s system_governance.components.%s must be present", configName, component)
		}
		assertURLPath(t, configName, component, "resilience_url", cfg.ResilienceURL, "/governance/resilience")
		assertURLPath(t, configName, component, "cache_url", cfg.CacheURL, "/governance/redis")
		if cfg.Timeout <= 0 {
			t.Fatalf("%s system_governance.components.%s.timeout must be positive", configName, component)
		}
	}
}

func assertURLPath(t *testing.T, configName, component, key, rawURL, wantPath string) {
	t.Helper()
	if strings.TrimSpace(rawURL) == "" {
		t.Fatalf("%s system_governance.components.%s.%s must not be empty", configName, component, key)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("%s system_governance.components.%s.%s must be parseable: %v", configName, component, key, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		t.Fatalf("%s system_governance.components.%s.%s must be absolute URL: %s", configName, component, key, rawURL)
	}
	if parsed.Path != wantPath {
		t.Fatalf("%s system_governance.components.%s.%s path = %q, want %q", configName, component, key, parsed.Path, wantPath)
	}
}

func stubSecureTLSFiles(t *testing.T, secure *genericoptions.SecureServingOptions) {
	t.Helper()
	if secure == nil || secure.BindPort == 0 {
		return
	}
	secure.TLS.CertFile = writeTempFile(t, "cert.pem")
	secure.TLS.KeyFile = writeTempFile(t, "key.pem")
}

func writeTempFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func completeAndValidate(t *testing.T, opts interface {
	Complete() error
	Validate() []error
}) {
	t.Helper()
	if err := opts.Complete(); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if errs := opts.Validate(); len(errs) > 0 {
		t.Fatalf("Validate() errors = %v", errs)
	}
}

func assertRedisFamilies(t *testing.T, runtimeOpts *genericoptions.RedisRuntimeOptions, families []redisruntime.Family) {
	t.Helper()
	if runtimeOpts == nil {
		t.Fatal("redis runtime options are nil")
	}
	for _, family := range families {
		route, ok := runtimeOpts.Families[string(family)]
		if !ok {
			t.Fatalf("redis runtime family %q is missing", family)
		}
		if route == nil || strings.TrimSpace(route.NamespaceSuffix) == "" {
			t.Fatalf("redis runtime family %q has empty namespace suffix", family)
		}
	}
}

func assertEventCatalogLoads(t *testing.T) {
	t.Helper()
	cfg, err := eventcatalog.Load(filepath.Join(repoRoot(t), "configs", "events.yaml"))
	if err != nil {
		t.Fatalf("Load events.yaml: %v", err)
	}
	if len(cfg.Events) == 0 || len(cfg.Topics) == 0 {
		t.Fatal("events.yaml must define topics and events")
	}
}

func workerEventConfigPath(worker *workeroptions.WorkerOptions) string {
	if worker != nil && worker.EventConfigPath != "" {
		return worker.EventConfigPath
	}
	return "configs/events.yaml"
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
