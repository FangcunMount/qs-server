package resilienceplane_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
)

func TestResiliencePathsDoNotImportPrometheusDirectly(t *testing.T) {
	root := repoRoot(t)
	paths := []string{
		"internal/pkg/middleware",
		"internal/pkg/backpressure",
		"internal/pkg/locklease/redisadapter",
		"internal/pkg/cacheplane",
		"internal/collection-server/application/answersheet",
		"internal/collection-server/infra/redisops",
		"internal/worker/handlers",
	}
	for _, rel := range paths {
		scanGoFiles(t, filepath.Join(root, rel), func(path string, file *ast.File) {
			for _, imported := range file.Imports {
				importPath := strings.Trim(imported.Path.Value, `"`)
				if strings.HasPrefix(importPath, "github.com/prometheus/") {
					t.Fatalf("%s imports %s; resilience metrics must go through internal/pkg/resilienceplane", path, importPath)
				}
			}
		})
	}
}

func TestBusinessCodeDoesNotImportComponentBaseLeaseDirectly(t *testing.T) {
	root := repoRoot(t)
	scanGoFiles(t, filepath.Join(root, "internal"), func(path string, file *ast.File) {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(rel, "internal/pkg/locklease/redisadapter/") {
			return
		}
		for _, imported := range file.Imports {
			if strings.Trim(imported.Path.Value, `"`) == "github.com/FangcunMount/component-base/pkg/redis/lease" {
				t.Fatalf("%s imports component-base redis lease directly; use internal/pkg/locklease port", rel)
			}
		}
	})
}

func TestBusinessCodeDoesNotImportLockLeaseRedisAdapterDirectly(t *testing.T) {
	root := repoRoot(t)
	allowed := map[string]struct{}{
		"internal/pkg/cacheplane/bootstrap":   {},
		"internal/pkg/locklease/redisadapter": {},
		"internal/apiserver/cachebootstrap":   {},
		"internal/pkg/resilienceplane":        {},
		"internal/pkg/configcontract":         {},
		"internal/collection-server/process":  {},
		"internal/worker/process":             {},
	}
	scanGoFiles(t, filepath.Join(root, "internal"), func(path string, file *ast.File) {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(rel, "_test.go") {
			return
		}
		for prefix := range allowed {
			if strings.HasPrefix(rel, prefix+"/") {
				return
			}
		}
		for _, imported := range file.Imports {
			if strings.Trim(imported.Path.Value, `"`) == "github.com/FangcunMount/qs-server/internal/pkg/locklease/redisadapter" {
				t.Fatalf("%s imports locklease redis adapter directly; use internal/pkg/locklease port", rel)
			}
		}
	})
}

func TestProductionCodeDoesNotImportTopLevelRedisPackages(t *testing.T) {
	root := repoRoot(t)
	scanGoFiles(t, filepath.Join(root, "internal"), func(path string, file *ast.File) {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(rel, "_test.go") {
			return
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if strings.HasPrefix(importPath, "github.com/FangcunMount/qs-server/internal/pkg/redis") {
				t.Fatalf("%s imports top-level redis package %s; use cacheplane, cachegovernance, locklease, or layer-local redisadapter", rel, importPath)
			}
		}
	})
}

func TestBusinessCodeDoesNotImportGoRedisDirectly(t *testing.T) {
	root := repoRoot(t)
	paths := []string{
		"internal/apiserver/application",
		"internal/apiserver/cachetarget",
		"internal/apiserver/domain",
		"internal/collection-server/application",
		"internal/worker/handlers",
	}
	for _, relRoot := range paths {
		scanGoFiles(t, filepath.Join(root, relRoot), func(path string, file *ast.File) {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(rel, "_test.go") {
				return
			}
			for _, imported := range file.Imports {
				if strings.Trim(imported.Path.Value, `"`) == "github.com/redis/go-redis/v9" {
					t.Fatalf("%s imports go-redis directly; business code should depend on cache, governance, lock, or ratelimit ports", rel)
				}
			}
		})
	}
}

func TestBackpressureIsNotConfiguredThroughPackageGlobals(t *testing.T) {
	root := repoRoot(t)
	paths := []string{
		"internal/pkg/database/mysql",
		"internal/apiserver/infra/mongo",
		"internal/apiserver/infra/iam",
		"internal/apiserver/process",
	}
	for _, rel := range paths {
		scanGoSourceFiles(t, filepath.Join(root, rel), func(path string, content string) {
			if strings.Contains(content, "SetLimiter(") {
				t.Fatalf("%s uses SetLimiter; backpressure must be injected explicitly", path)
			}
			if strings.Contains(content, "var limiter *backpressure") {
				t.Fatalf("%s declares package-level backpressure limiter", path)
			}
			if strings.Contains(content, "func acquire(ctx context.Context)") {
				t.Fatalf("%s declares package-level acquire helper", path)
			}
		})
	}
}

func TestRateLimitEntrypointsUseDecisionAdapter(t *testing.T) {
	root := repoRoot(t)
	paths := []string{
		"internal/apiserver/transport/rest",
		"internal/collection-server/transport/rest",
	}
	for _, rel := range paths {
		scanGoSourceFiles(t, filepath.Join(root, rel), func(path string, content string) {
			if strings.Contains(content, "resilienceplane.Observe(") {
				t.Fatalf("%s observes rate limit directly; use internal/pkg/ratelimit decision adapter", path)
			}
			if strings.Contains(content, `Header("Retry-After"`) {
				t.Fatalf("%s writes Retry-After directly; use internal/pkg/ratelimit decision adapter", path)
			}
		})
	}
}

func TestLockLeaseSpecsHaveResilienceSemantics(t *testing.T) {
	specs := []locklease.Spec{
		locklease.Specs.AnswersheetProcessing,
		locklease.Specs.PlanSchedulerLeader,
		locklease.Specs.StatisticsSyncLeader,
		locklease.Specs.StatisticsSync,
		locklease.Specs.BehaviorPendingReconcile,
		locklease.Specs.CollectionSubmit,
	}
	for _, spec := range specs {
		if spec.Name == "" {
			t.Fatal("lock spec name must not be empty")
		}
		if strings.TrimSpace(spec.Description) == "" {
			t.Fatalf("lock spec %q must have a semantic description", spec.Name)
		}
		if spec.DefaultTTL <= 0*time.Second {
			t.Fatalf("lock spec %q ttl = %s, want positive", spec.Name, spec.DefaultTTL)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
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

func scanGoFiles(t *testing.T, root string, visit func(path string, file *ast.File)) {
	t.Helper()
	fset := token.NewFileSet()
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		visit(path, file)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func scanGoSourceFiles(t *testing.T, root string, visit func(path string, content string)) {
	t.Helper()
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		visit(path, string(bytes))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
