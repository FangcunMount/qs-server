package configcontract

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	basegrpc "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	collectionconfig "github.com/FangcunMount/qs-server/internal/collection-server/config"
	collectiongrpcclient "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	collectionoptions "github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
	workergrpcclient "github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	workeroptions "github.com/FangcunMount/qs-server/internal/worker/options"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

func TestAPIServerDevProdConfigContracts(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"apiserver.dev.yaml", "apiserver.prod.yaml"} {
		t.Run(name, func(t *testing.T) {
			opts := apiserveroptions.NewOptions()
			loadConfig(t, filepath.Join(repoRoot(t), "configs", name), opts)
			prepareDelegatedSubjectContract(t, name, opts.DelegatedSubject)
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
			assertRenewalMode(t, name, opts.LockLease)
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
			assertStatisticsCacheContract(t, name, opts.Cache)
			assertIAMJWKSURLContract(t, "apiserver", name, opts.IAMOptions)
			assertAPIServerGRPCTrustContract(t, name, opts)
			assertEventCatalogLoads(t)
		})
	}
}

func assertStatisticsCacheContract(t *testing.T, configName string, opts *apiserveroptions.CacheOptions) {
	t.Helper()
	if opts == nil || opts.Capabilities == nil || opts.Capabilities.Statistics == nil || opts.Capabilities.Statistics.Query == nil {
		t.Fatalf("%s statistics.query cache capability must be traceable", configName)
	}
	if got := opts.Capabilities.Statistics.Query.TTL; got != 26*time.Hour {
		t.Fatalf("%s statistics.query TTL = %s, want 26h", configName, got)
	}
	if opts.Governance == nil || opts.Governance.StatisticsWarmup == nil {
		t.Fatalf("%s statistics warmup config must be traceable", configName)
	}
	want := []string{"latest_complete_day", "7d", "30d"}
	got := opts.Governance.StatisticsWarmup.OverviewPresets
	if len(got) != len(want) {
		t.Fatalf("%s statistics warmup presets = %v, want %v", configName, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s statistics warmup presets = %v, want %v", configName, got, want)
		}
	}
}

func TestCollectionDevProdConfigContracts(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"collection-server.dev.yaml", "collection-server.prod.yaml"} {
		t.Run(name, func(t *testing.T) {
			opts := collectionoptions.NewOptions()
			loadConfig(t, filepath.Join(repoRoot(t), "configs", name), opts)
			prepareDelegatedSubjectContract(t, name, opts.DelegatedSubject)
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
			assertRenewalMode(t, name, opts.LockLease)
			if opts.RateLimit == nil || !opts.RateLimit.Enabled {
				t.Fatal("collection rate limit config must be traceable and enabled by default")
			}
			if opts.IAMOptions == nil || opts.IAMOptions.ServiceAuth == nil {
				t.Fatal("collection IAM service auth config must be traceable")
			}
			assertCollectionGRPCClientIdentityContract(t, name, opts.GRPCClient)
			assertIAMJWKSURLContract(t, "collection", name, opts.IAMOptions)
		})
	}
}

func prepareDelegatedSubjectContract(t *testing.T, configName string, opts *delegatedsubject.Options) {
	t.Helper()
	if !strings.Contains(configName, ".prod.") {
		return
	}
	if opts == nil || !opts.Enabled {
		t.Fatalf("%s delegated-subject must be present and enabled", configName)
	}
	if opts.TTL != delegatedsubject.DefaultTTL {
		t.Fatalf("%s delegated-subject ttl = %s, want %s", configName, opts.TTL, delegatedsubject.DefaultTTL)
	}
	if opts.CurrentKey != "" || opts.PreviousKey != "" {
		t.Fatalf("%s delegated-subject keys must not be committed to config", configName)
	}
	// Production injects this value through the service-prefixed
	// *_DELEGATED_SUBJECT_CURRENT_KEY environment variable.
	// Supply a non-secret test value only after proving the file itself is empty.
	opts.CurrentKey = "config-contract-test-key"
}

func assertAPIServerGRPCTrustContract(t *testing.T, configName string, opts *apiserveroptions.Options) {
	t.Helper()
	if strings.Contains(configName, ".dev.") {
		found := make(map[string]bool, len(opts.GRPCOptions.MTLS.AllowedCNs))
		for _, allowedCN := range opts.GRPCOptions.MTLS.AllowedCNs {
			found[allowedCN] = true
		}
		for _, required := range []string{
			serviceidentity.CollectionServerCertificateCommonName,
			serviceidentity.WorkerCertificateCommonName,
		} {
			if !found[required] {
				t.Fatalf("%s grpc mTLS allowed CNs must contain %q", configName, required)
			}
		}
		return
	}
	if !strings.Contains(configName, ".prod.") {
		return
	}
	if opts.GRPCOptions == nil || opts.GRPCOptions.ACL == nil || !opts.GRPCOptions.ACL.Enabled {
		t.Fatalf("%s grpc ACL must be enabled", configName)
	}
	if opts.GRPCOptions.ACL.DefaultPolicy != "deny" {
		t.Fatalf("%s grpc ACL default policy = %q, want deny", configName, opts.GRPCOptions.ACL.DefaultPolicy)
	}
	if opts.GRPCOptions.ACL.ConfigFile != "configs/grpc-acl.prod.yaml" {
		t.Fatalf("%s grpc ACL config file = %q", configName, opts.GRPCOptions.ACL.ConfigFile)
	}
	data, err := os.ReadFile(filepath.Join(repoRoot(t), opts.GRPCOptions.ACL.ConfigFile))
	if err != nil {
		t.Fatalf("read production grpc ACL: %v", err)
	}
	assertExactGRPCACLConfig(t, opts.GRPCOptions.ACL.ConfigFile, data)
}

func TestGRPCACLFilesMatchCanonicalClientContracts(t *testing.T) {
	t.Parallel()

	for _, configName := range []string{"grpc-acl.prod.yaml", "grpc-acl.example.yaml"} {
		configName := configName
		t.Run(configName, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(filepath.Join(repoRoot(t), "configs", configName))
			if err != nil {
				t.Fatalf("read %s: %v", configName, err)
			}
			assertExactGRPCACLConfig(t, configName, data)
		})
	}
}

func assertExactGRPCACLConfig(t *testing.T, configName string, data []byte) {
	t.Helper()

	var aclConfig basegrpc.ACLConfig
	if err := yaml.Unmarshal(data, &aclConfig); err != nil {
		t.Fatalf("parse %s: %v", configName, err)
	}
	if aclConfig.DefaultPolicy != "deny" {
		t.Fatalf("%s default_policy = %q, want deny", configName, aclConfig.DefaultPolicy)
	}
	if len(aclConfig.Services) != 2 {
		t.Fatalf("%s service rule count = %d, want 2", configName, len(aclConfig.Services))
	}
	expectedMethodsByIdentity := map[string][]string{
		serviceidentity.CollectionServerCertificateCommonName: collectiongrpcclient.ACLAllowedMethods(),
		serviceidentity.WorkerCertificateCommonName:           workergrpcclient.ACLAllowedMethods(),
	}
	seenIdentities := make(map[string]struct{}, len(aclConfig.Services))
	for _, service := range aclConfig.Services {
		if service == nil {
			t.Fatalf("%s contains a null service rule", configName)
		}
		expectedMethods, ok := expectedMethodsByIdentity[service.ServiceName]
		if !ok {
			t.Fatalf("%s contains non-canonical identity %q", configName, service.ServiceName)
		}
		if _, duplicate := seenIdentities[service.ServiceName]; duplicate {
			t.Fatalf("%s duplicates identity %q", configName, service.ServiceName)
		}
		seenIdentities[service.ServiceName] = struct{}{}
		if !service.Enabled {
			t.Fatalf("%s identity %q must be enabled", configName, service.ServiceName)
		}
		if len(service.DeniedMethods) != 0 || len(service.MethodPermissions) != 0 {
			t.Fatalf("%s identity %q must use only exact allowed_methods", configName, service.ServiceName)
		}
		assertExactStringSet(t, configName+" "+service.ServiceName+" allowed_methods", service.AllowedMethods, expectedMethods)
	}
	for identity := range expectedMethodsByIdentity {
		if _, ok := seenIdentities[identity]; !ok {
			t.Fatalf("%s missing canonical identity %q", configName, identity)
		}
	}
}

func assertExactStringSet(t *testing.T, name string, actual, expected []string) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("%s count = %d, want %d", name, len(actual), len(expected))
	}
	actualSet := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		if _, duplicate := actualSet[value]; duplicate {
			t.Fatalf("%s contains duplicate %q", name, value)
		}
		actualSet[value] = struct{}{}
	}
	for _, value := range expected {
		if _, ok := actualSet[value]; !ok {
			t.Fatalf("%s missing %q", name, value)
		}
	}
}

func TestAPIServerImageIncludesProductionGRPCACL(t *testing.T) {
	dockerfile, err := os.ReadFile(filepath.Join(repoRoot(t), "build", "docker", "Dockerfile.qs-apiserver"))
	if err != nil {
		t.Fatalf("read apiserver Dockerfile: %v", err)
	}
	const required = "COPY --chown=www:www configs/grpc-acl.prod.yaml /app/configs/grpc-acl.prod.yaml"
	if !strings.Contains(string(dockerfile), required) {
		t.Fatalf("apiserver Dockerfile must contain %q", required)
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
			assertRenewalMode(t, name, opts.LockLease)
			if cfg.Messaging == nil || cfg.Messaging.Provider == "" {
				t.Fatal("worker messaging config must be traceable")
			}
			if cfg.Worker == nil || cfg.Worker.Concurrency <= 0 {
				t.Fatal("worker runtime config must define positive concurrency")
			}
			assertWorkerGRPCClientIdentityContract(t, name, opts.GRPC)
			if workerEventConfigPath(cfg.Worker) != "configs/events.yaml" {
				t.Fatalf("worker event config fallback = %q, want configs/events.yaml", workerEventConfigPath(cfg.Worker))
			}
			assertEventCatalogLoads(t)
		})
	}
}

func assertCollectionGRPCClientIdentityContract(
	t *testing.T,
	configName string,
	opts *collectionoptions.GRPCClientOptions,
) {
	t.Helper()
	if opts == nil || opts.Insecure {
		t.Fatalf("%s collection gRPC client must use mTLS", configName)
	}
	if strings.TrimSpace(opts.TLSCAFile) == "" ||
		strings.TrimSpace(opts.TLSCertFile) == "" ||
		strings.TrimSpace(opts.TLSKeyFile) == "" {
		t.Fatalf("%s collection gRPC client mTLS files must be configured", configName)
	}
	if opts.TLSServerName != serviceidentity.APIServerCertificateCommonName {
		t.Fatalf(
			"%s collection grpc server name = %q, want %q",
			configName,
			opts.TLSServerName,
			serviceidentity.APIServerCertificateCommonName,
		)
	}
}

func assertWorkerGRPCClientIdentityContract(
	t *testing.T,
	configName string,
	opts *workeroptions.GRPCOptions,
) {
	t.Helper()
	if opts == nil || opts.Insecure {
		t.Fatalf("%s worker gRPC client must use mTLS", configName)
	}
	if strings.TrimSpace(opts.TLSCAFile) == "" ||
		strings.TrimSpace(opts.TLSCertFile) == "" ||
		strings.TrimSpace(opts.TLSKeyFile) == "" {
		t.Fatalf("%s worker gRPC client mTLS files must be configured", configName)
	}
	if opts.TLSServerName != serviceidentity.APIServerCertificateCommonName {
		t.Fatalf(
			"%s worker grpc server name = %q, want %q",
			configName,
			opts.TLSServerName,
			serviceidentity.APIServerCertificateCommonName,
		)
	}
}

func TestRemoteDeployChecksCanonicalGRPCCertificateIdentities(t *testing.T) {
	t.Parallel()

	script, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "cd", "remote-deploy.sh"))
	if err != nil {
		t.Fatalf("read remote deploy script: %v", err)
	}
	content := string(script)
	for _, required := range []string{
		"setup_grpc_certs qs-apiserver " + serviceidentity.APIServerCertificateCommonName,
		"setup_grpc_certs qs-collection-server " + serviceidentity.CollectionServerCertificateCommonName,
		"setup_grpc_certs qs-worker " + serviceidentity.WorkerCertificateCommonName,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("remote deploy script must contain %q", required)
		}
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
		if component == "worker" && cfg.DiscoveryMode() != "single" {
			t.Fatalf("%s worker governance discovery = %q, want single", configName, cfg.DiscoveryMode())
		}
		if component == "collection-server" {
			if configName == "apiserver.prod.yaml" {
				if cfg.DiscoveryMode() != "dns" || cfg.RequiredInstances() != 2 {
					t.Fatalf("%s collection governance discovery = %q minimum=%d, want dns/2", configName, cfg.DiscoveryMode(), cfg.RequiredInstances())
				}
				if parsed, _ := url.Parse(cfg.ResilienceURL); parsed == nil || parsed.Hostname() != "qs-collection-server" {
					t.Fatalf("%s collection governance must use stable qs-collection-server DNS: %s", configName, cfg.ResilienceURL)
				}
			} else if cfg.DiscoveryMode() != "single" {
				t.Fatalf("%s collection governance discovery = %q, want single", configName, cfg.DiscoveryMode())
			}
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

func assertRenewalMode(t *testing.T, configName string, opts *genericoptions.LockLeaseOptions) {
	t.Helper()
	if opts == nil {
		t.Fatal("lock_lease config must be explicit in dev/prod config")
	}
	wantEnabled := strings.Contains(configName, ".dev.")
	if opts.RenewalEnabled != wantEnabled {
		t.Fatalf("lock_lease.renewal_enabled = %v, want %v for %s", opts.RenewalEnabled, wantEnabled, configName)
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
