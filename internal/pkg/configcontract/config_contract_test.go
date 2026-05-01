package configcontract

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	collectionconfig "github.com/FangcunMount/qs-server/internal/collection-server/config"
	collectionoptions "github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
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
			assertRedisFamilies(t, opts.RedisRuntime, []cacheplane.Family{
				cacheplane.FamilyStatic,
				cacheplane.FamilyObject,
				cacheplane.FamilyQuery,
				cacheplane.FamilyMeta,
				cacheplane.FamilyRank,
				cacheplane.FamilySDK,
				cacheplane.FamilyLock,
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
			assertRedisFamilies(t, opts.RedisRuntime, []cacheplane.Family{
				cacheplane.FamilyOps,
				cacheplane.FamilyLock,
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
			assertRedisFamilies(t, opts.RedisRuntime, []cacheplane.Family{
				cacheplane.FamilyLock,
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

func loadConfig(t *testing.T, path string, target any) {
	t.Helper()
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig(%s) error = %v", path, err)
	}
	if err := v.Unmarshal(target); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", path, err)
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

func assertRedisFamilies(t *testing.T, runtimeOpts *genericoptions.RedisRuntimeOptions, families []cacheplane.Family) {
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
