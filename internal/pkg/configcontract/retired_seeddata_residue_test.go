package configcontract

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestRetiredSeeddataResidueIsConfinedToCompatibilityContracts(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	needles := []string{
		"qs_historical_context_secret",
		"historicalexecutioncontext",
		"historicalcontext",
		"historicalrequestedat",
		"historicalsignature",
		"historical_context",
		"historical_seed",
		"historicalseed",
		"historical-seed",
		"historical seed",
		"x-qs-historical-",
		"seed_backfill_",
		"seed backfill",
		"historical-backfill",
		"resume-from",
		"mongodbindexscript",
		"db.scales",
	}

	var violations []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if shouldSkipRetiredSeeddataScanDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isRetiredSeeddataScanSource(rel) {
			return nil
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for lineNo, sourceLine := range strings.Split(string(contents), "\n") {
			line := strings.ToLower(sourceLine)
			for _, needle := range needles {
				if strings.Contains(line, needle) && !isAllowedRetiredSeeddataResidue(rel, needle) {
					violations = append(violations, fmt.Sprintf("%s:%d contains %q", rel, lineNo+1, needle))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan retired seeddata residue: %v", err)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("retired seeddata behavior escaped its compatibility allowlist:\n%s", strings.Join(violations, "\n"))
	}
}

func shouldSkipRetiredSeeddataScanDir(rel string) bool {
	switch rel {
	case ".git", ".cursor", ".codex-python-wrapper", "bin", "logs", "repair_backups", "tmp", "docs/_archive":
		return true
	default:
		return false
	}
}

func isRetiredSeeddataScanSource(rel string) bool {
	base := filepath.Base(rel)
	if base == "Dockerfile" || base == "Makefile" {
		return true
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".go", ".proto", ".yaml", ".yml", ".json", ".sql", ".sh", ".md", ".js", ".ts", ".tsx", ".toml", ".env", ".example":
		return true
	default:
		return false
	}
}

func isAllowedRetiredSeeddataResidue(rel, needle string) bool {
	if rel == ".github/workflows/db-ops.yml" {
		// Production status only checks whether retired tables still exist; it
		// must retain their exact names without reintroducing executable paths.
		return needle == "seed_backfill_"
	}
	if rel == "internal/pkg/configcontract/retired_seeddata_residue_test.go" ||
		rel == "internal/pkg/middleware/retired_historical_seed.go" ||
		rel == "internal/pkg/middleware/retired_historical_seed_test.go" ||
		rel == "internal/pkg/server/genericapiserver.go" ||
		rel == "internal/pkg/server/historical_seed_guard_test.go" ||
		rel == "internal/apiserver/process/transport_bootstrap_test.go" ||
		rel == "internal/collection-server/process/transport_bootstrap_test.go" ||
		rel == "internal/apiserver/router_matrix_test.go" ||
		rel == "internal/pkg/eventing/payload/wire_contract_test.go" ||
		rel == "internal/worker/handlers/legacy_event_contract_test.go" ||
		rel == "internal/pkg/migration/driver_mongo.go" ||
		rel == "internal/pkg/migration/historical_seed_stage_contract_test.go" ||
		rel == "internal/pkg/migration/retire_seeddata_contract_test.go" ||
		rel == "internal/pkg/migration/retire_seeddata_integration_test.go" ||
		rel == "docs/05-决策记录/01-架构决策总表.md" {
		return true
	}
	if strings.HasPrefix(rel, "internal/pkg/migration/migrations/") {
		return true
	}
	for _, prefix := range []string{
		"api/grpc/proto/actor/",
		"api/grpc/proto/answersheet/",
		"api/grpc/proto/evaluation/",
		"api/grpc/proto/interpretation/",
		"api/grpc/gen/actor/",
		"api/grpc/gen/answersheet/",
		"api/grpc/gen/evaluation/",
		"api/grpc/gen/interpretation/",
	} {
		if strings.HasPrefix(rel, prefix) {
			// Generated descriptors retain the reserved field name, but no
			// executable historical type or protocol marker may return.
			return needle == "historical_context"
		}
	}
	return false
}
