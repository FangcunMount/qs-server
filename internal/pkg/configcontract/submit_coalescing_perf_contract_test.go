package configcontract

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubmitCoalescingPerfContract(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	script := readContractFile(t, filepath.Join(root, "scripts", "perf", "k6-submit-coalescing.js"))
	for _, required := range []string{
		"COALESCING_SCENARIO",
		"PERF_ISOLATED_ENV",
		"healthy",
		"conflict",
		"redis_lock_failure",
		"redis_signal_failure",
		"redis_unavailable",
		"owner",
		"contender_signaled",
		"contender_timeout",
		"degraded_open",
		"signal_error",
		"lease_acquire",
		"collectionInstanceActivity",
		"http.expectedStatuses(202, 409)",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("SubmitCoalescer k6 script must contain %q", required)
		}
	}

	runner := readContractFile(t, filepath.Join(root, "scripts", "perf", "run-submit-coalescing.sh"))
	for _, required := range []string{
		`COLLECTION_COMPOSE_SERVICE="${COLLECTION_COMPOSE_SERVICE:-server}"`,
		"label=com.docker.compose.project=${COLLECTION_COMPOSE_PROJECT}",
		"label=com.docker.compose.service=${COLLECTION_COMPOSE_SERVICE}",
		"EXPECTED_COLLECTION_REPLICAS",
		"COLLECTION_NETWORK",
		"COLLECTION_BASE_URLS",
		"COLLECTION_METRICS_URLS",
		"APISERVER_METRICS_URL",
		"k6-summary.json",
		`if [[ "$VERIFY_METRICS" == "true" && "$PERF_ISOLATED_ENV" != "true" ]]`,
	} {
		if !strings.Contains(runner, required) {
			t.Errorf("SubmitCoalescer runner must contain %q", required)
		}
	}

	makefile := readContractFile(t, filepath.Join(root, "Makefile"))
	for _, required := range []string{
		"run-submit-coalescing.sh",
		"bash -n $(PERF_SCRIPT_DIR)/run-submit-coalescing.sh",
		"k6 inspect $(PERF_SCRIPT_DIR)/k6-answersheet-submit.js",
	} {
		if !strings.Contains(makefile, required) {
			t.Errorf("Makefile perf contract must contain %q", required)
		}
	}

	doc := readContractFile(t, filepath.Join(root, "docs", "03-基础设施", "concurrency", "70-可观测性-压测与验收.md"))
	for _, required := range []string{
		"run-submit-coalescing.sh",
		"COALESCING_SCENARIO=healthy",
		"COALESCING_SCENARIO=conflict",
		"COALESCING_SCENARIO=redis_lock_failure",
		"COALESCING_SCENARIO=redis_signal_failure",
		"COALESCING_SCENARIO=redis_unavailable",
	} {
		if !strings.Contains(doc, required) {
			t.Errorf("SubmitCoalescer perf documentation must contain %q", required)
		}
	}
}

func TestCollectionRuntimeAcceptanceEntryContract(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	runner := readContractFile(t, filepath.Join(root, "scripts", "perf", "run-collection-runtime-acceptance.sh"))
	for _, required := range []string{
		"healthy-smoke",
		"healthy",
		"degraded-low",
		"degraded-global",
		"degraded-user",
		"recovery",
		`VERIFY_METRICS=false`,
		`PERF_ISOLATED_ENV=true`,
		`REDIS_FAILURE_CONFIRMED=true`,
		`REDIS_RECOVERY_CONFIRMED`,
		`strategy="local_fallback"`,
		`strategy="redis"`,
	} {
		if !strings.Contains(runner, required) {
			t.Errorf("collection runtime acceptance entry must contain %q", required)
		}
	}
	if strings.Contains(runner, "docker stop") || strings.Contains(runner, "docker restart") {
		t.Error("collection runtime acceptance must not mutate Redis or container lifecycle")
	}

	makefile := readContractFile(t, filepath.Join(root, "Makefile"))
	for _, required := range []string{
		"perf-collection-runtime-healthy-smoke",
		"perf-collection-runtime-healthy",
		"perf-collection-runtime-degraded-low",
		"perf-collection-runtime-degraded-global",
		"perf-collection-runtime-degraded-user",
		"perf-collection-runtime-recovery",
		"bash -n $(PERF_SCRIPT_DIR)/run-collection-runtime-acceptance.sh",
	} {
		if !strings.Contains(makefile, required) {
			t.Errorf("Makefile runtime acceptance contract must contain %q", required)
		}
	}
}

func TestPerfScriptsDoNotEmbedProductionCredentialsOrLegacyCollectionAddress(t *testing.T) {
	t.Parallel()

	perfRoot := filepath.Join(repoRoot(t), "scripts", "perf")
	err := filepath.WalkDir(perfRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".js" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		if strings.Contains(text, "eyJhbGciOi") {
			t.Errorf("%s contains an embedded JWT", path)
		}
		if strings.Contains(text, "47.94.204.124") {
			t.Errorf("%s contains the legacy production IP", path)
		}
		if strings.Contains(text, "127.0.0.1:8082") {
			t.Errorf("%s contains the retired collection port 8082", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk perf scripts: %v", err)
	}

	submitScript := readContractFile(t, filepath.Join(perfRoot, "k6-answersheet-submit.js"))
	for _, required := range []string{"SUBMIT_PAYLOAD_JSON", "idempotency_key", "status === 202", "answersheet_id"} {
		if !strings.Contains(submitScript, required) {
			t.Errorf("standalone submit script must contain %q", required)
		}
	}
}

func readContractFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
