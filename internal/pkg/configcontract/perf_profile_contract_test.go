package configcontract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestPerfProfilesMatchAdmissionContract(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "scripts/perf/qs-perf.config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		QPSProfile string `json:"qpsProfile"`
		ReportMode string `json:"reportMode"`
		Profiles   map[string]struct {
			QPS    map[string]float64 `json:"qps"`
			VUsers json.RawMessage    `json:"vusers"`
		} `json:"qpsProfiles"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if config.ReportMode != "websocket" {
		t.Fatalf("default reportMode = %q, want websocket", config.ReportMode)
	}
	if config.QPSProfile != "smoke_4" {
		t.Fatalf("default qpsProfile = %q, want smoke_4", config.QPSProfile)
	}
	wantProfiles := []string{"admission_300", "experience_60", "smoke_4"}
	gotProfiles := make([]string, 0, len(config.Profiles))
	for name, profile := range config.Profiles {
		gotProfiles = append(gotProfiles, name)
		if len(profile.VUsers) != 0 {
			t.Fatalf("profile %s still maintains manual vusers", name)
		}
	}
	sort.Strings(gotProfiles)
	if !reflect.DeepEqual(gotProfiles, wantProfiles) {
		t.Fatalf("profile keys = %v, want %v", gotProfiles, wantProfiles)
	}
	wantAdmission := map[string]float64{
		"medicalQuery": 80, "personalityQuery": 40, "questionnaireQuery": 13,
		"personalityQuestionnaireQuery": 13, "medicalSubmit": 19, "personalitySubmit": 5,
		"medicalWaitReport": 70, "behaviorWaitReport": 10, "personalityWaitReport": 20,
		"stats": 29, "chainProbe": 1,
	}
	if !reflect.DeepEqual(config.Profiles["admission_300"].QPS, wantAdmission) {
		t.Fatalf("admission_300 qps = %#v, want %#v", config.Profiles["admission_300"].QPS, wantAdmission)
	}

	sopRaw, err := os.ReadFile(filepath.Join(root, "docs/04-接口与运维/11-300QPS混合场景压测SOP.md"))
	if err != nil {
		t.Fatal(err)
	}
	sop := string(sopRaw)
	for _, want := range []string{
		"make perf-run PLAN=admission",
		"capacity 120 QPS",
		"恢复证据门",
		"受理 TPS",
		"P50/P90/P95/P99",
	} {
		if !strings.Contains(sop, want) {
			t.Fatalf("SOP missing capacity contract fragment %q", want)
		}
	}
	for _, retired := range []string{"make perf-mixed", "make perf-pretest", "make perf-sync-vusers"} {
		if strings.Contains(sop, retired) {
			t.Fatalf("SOP still contains retired command %q", retired)
		}
	}
}

func TestPerfThresholdTiersAndOperationMetrics(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "scripts/perf/qs-perf.config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		ThresholdTier string `json:"thresholdTier"`
		Profiles      map[string]struct {
			ThresholdTier string             `json:"thresholdTier"`
			QPS           map[string]float64 `json:"qps"`
		} `json:"qpsProfiles"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if config.ThresholdTier != "none" {
		t.Fatalf("default thresholdTier = %q, want none", config.ThresholdTier)
	}
	experience := config.Profiles["experience_60"]
	if experience.ThresholdTier != "experience" {
		t.Fatalf("experience_60 thresholdTier = %q, want experience", experience.ThresholdTier)
	}
	var experienceQPS float64
	for _, qps := range experience.QPS {
		experienceQPS += qps
	}
	if experienceQPS != 60 {
		t.Fatalf("experience_60 total qps = %v, want 60", experienceQPS)
	}
	admission := config.Profiles["admission_300"]
	if admission.ThresholdTier != "protection" {
		t.Fatalf("admission_300 thresholdTier = %q, want protection", admission.ThresholdTier)
	}
	var admissionQPS float64
	for _, qps := range admission.QPS {
		admissionQPS += qps
	}
	if admissionQPS != 300 {
		t.Fatalf("admission_300 total qps = %v, want 300", admissionQPS)
	}
	for _, key := range []string{"medicalSubmit", "personalitySubmit", "medicalWaitReport", "behaviorWaitReport", "personalityWaitReport"} {
		if admission.QPS[key] <= 0 {
			t.Fatalf("admission_300 missing split qps %q", key)
		}
	}

	files := []struct {
		path  string
		wants []string
	}{
		{
			path: "scripts/perf/k6/lib/metrics.js",
			wants: []string{
				"medical_answer_submit_duration",
				"personality_answer_submit_success_rate",
				"statistics_overview_duration",
				"statistics_content_batch_duration",
				"report_ws_connect_duration",
				"report_ws_first_message_latency",
				"report_ws_subscribe_to_first_message_latency",
				"dropped_iterations{scenario:",
				"experience: {",
				"query: [300, 500]",
				"statistics: [700, 1500]",
				"protection: {",
				"query: [500, 1200]",
			},
		},
		{
			path:  "scripts/perf/k6/lib/http.js",
			wants: []string{"X-Request-ID", "X-Perf-Run-ID"},
		},
		{
			path:  "scripts/perf/k6/mixed.js",
			wants: []string{"'med'", "'p(95)'", "'p(99)'", "'max'"},
		},
		{
			path:  "scripts/perf/k6/lib/config.js",
			wants: []string{"resolveArrivalVuserDefaults", "expectedLatencySeconds", "timeoutSeconds", "headroom"},
		},
		{
			path: "scripts/perf/perfctl/types.go",
			wants: []string{
				"qs-perf-report/v1", "accepted_tps_by_model", "completed_tps_by_model",
				"completion_window", "p50", "p90", "p95", "p99", "max", "success_rate", "error_rate", "timeout_rate",
			},
		},
		{
			path:  "internal/pkg/retryobservability/metrics.go",
			wants: []string{"qs_retry_layer_attempt_total", "layer", "component", "attempt_class", "origin", "outcome"},
		},
	}
	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(root, file.path))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range file.wants {
			if !strings.Contains(string(content), want) {
				t.Fatalf("%s missing redesigned perf contract fragment %q", file.path, want)
			}
		}
	}
}

func TestPerfMakefileHasSingleMainEntry(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "perf-run: perf-ensure-config") {
		t.Fatal("Makefile is missing the unified perf-run entry")
	}
	if !strings.Contains(text, "$(MAKE) perf-preflight && \\") {
		t.Fatal("perf-run must stop when perf-preflight fails")
	}
	for _, retired := range []string{
		"perf-k6:", "perf-smoke:", "perf-pretest60:", "perf-mixed140:",
		"perf-mixed280-models:", "perf-admission300:", "perf-sync-vusers:",
	} {
		if strings.Contains(text, retired) {
			t.Fatalf("Makefile still contains retired perf entry %q", retired)
		}
	}
}

func TestPerfPathsMatchCurrentRuntimeContract(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "scripts/perf/qs-perf.config.example.json"))
	if err != nil {
		t.Fatal(err)
	}

	for _, retired := range []string{
		"/api/v2/statistics/system",
		"/api/v2/statistics/questionnaires/",
	} {
		if strings.Contains(string(raw), retired) {
			t.Fatalf("perf config still references retired statistics route %q", retired)
		}
	}

	var config struct {
		Paths struct {
			StatisticsContentBatch string `json:"statisticsContentBatch"`
			BehaviorReportStatus   string `json:"behaviorReportStatus"`
			BehaviorReport         string `json:"behaviorReport"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}

	if config.Paths.StatisticsContentBatch != "/api/v2/statistics/contents/batch" {
		t.Fatalf("statisticsContentBatch = %q, want current batch endpoint", config.Paths.StatisticsContentBatch)
	}
	if config.Paths.BehaviorReportStatus != "/api/v1/behavior-assessments/{assessment_id}/report-status?testee_id={testee_id}" {
		t.Fatalf("behaviorReportStatus = %q, want current behavior report-status endpoint", config.Paths.BehaviorReportStatus)
	}
	if config.Paths.BehaviorReport != "/api/v1/behavior-assessments/{assessment_id}/report?testee_id={testee_id}" {
		t.Fatalf("behaviorReport = %q, want current behavior report endpoint", config.Paths.BehaviorReport)
	}
}

func TestPerfStatisticsContentBatchUsesKindContract(t *testing.T) {
	root := repoRoot(t)
	files := []struct {
		path    string
		wants   []string
		rejects []string
	}{
		{
			path:    "scripts/perf/k6/scenarios/statistics.js",
			wants:   []string{"kind: 'questionnaire'", "kind: 'scale'"},
			rejects: []string{"type: 'questionnaire'", "type: 'scale'"},
		},
		{
			path:    "scripts/perf/check-token-preflight.sh",
			wants:   []string{`\"kind\":\"scale\"`, `[[ "$status" =~ ^2[0-9][0-9]$ ]]`},
			rejects: []string{`\"type\":\"scale\"`},
		},
	}
	for _, file := range files {
		raw, err := os.ReadFile(filepath.Join(root, file.path))
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		for _, want := range file.wants {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing Statistics contract fragment %q", file.path, want)
			}
		}
		for _, reject := range file.rejects {
			if strings.Contains(text, reject) {
				t.Fatalf("%s still contains retired Statistics contract fragment %q", file.path, reject)
			}
		}
	}
}

func TestPerfSyncMigratesRetiredRuntimePaths(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is required by the perf profile sync script")
	}
	root := repoRoot(t)
	local := filepath.Join(t.TempDir(), "qs-perf.config.json")
	stale := `{
  "paths": {
    "medicalQuery": [
      "/api/v1/scales?page=1&page_size=20&status=published",
      "/api/v1/scales/categories",
      "/api/v1/scales/hot?limit=5",
      "/api/v1/scales/{scale_code}"
    ],
    "statistics": ["/api/v1/statistics/overview?preset=7d"],
    "statisticsContentBatch": "/api/v1/statistics/contents/batch"
  },
  "qpsProfiles": {
    "smoke_4": {
      "paths": {
        "questionnaireQuery": ["/api/v1/scales/{scale_code}"]
      }
    },
    "pretest_60": {
      "qps": {"query": 60}
    }
  }
}`
	if err := os.WriteFile(local, []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "scripts/perf/sync-profiles-from-example.sh")
	example := filepath.Join(root, "scripts/perf/qs-perf.config.example.json")
	cmd := exec.Command("bash", script, local, example)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sync perf profiles: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, retired := range []string{"/api/v1/scales", "/api/v1/statistics/"} {
		if strings.Contains(text, retired) {
			t.Fatalf("synced config still contains retired path prefix %q", retired)
		}
	}
	for _, current := range []string{
		"/api/v1/assessment-models?kind=scale&page=1&page_size=20",
		"/api/v1/assessment-models/options?kind=scale",
		"/api/v1/assessment-models/hot?kind=scale&limit=5",
		"/api/v1/assessment-models/{scale_code}",
		"/api/v2/statistics/overview?preset=7d",
		"/api/v2/statistics/contents/batch",
	} {
		if !strings.Contains(text, current) {
			t.Fatalf("synced config missing current path %q", current)
		}
	}
	var synced struct {
		QPSProfile string                     `json:"qpsProfile"`
		Profiles   map[string]json.RawMessage `json:"qpsProfiles"`
	}
	if err := json.Unmarshal(raw, &synced); err != nil {
		t.Fatal(err)
	}
	if synced.QPSProfile != "smoke_4" || len(synced.Profiles) != 3 || synced.Profiles["pretest_60"] != nil {
		t.Fatalf("sync did not retire old profiles: default=%q keys=%v", synced.QPSProfile, mapsKeys(synced.Profiles))
	}
}

func mapsKeys(values map[string]json.RawMessage) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
