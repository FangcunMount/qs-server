package configcontract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPerfProfilesMatchSOPCapacityContract(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "scripts/perf/qs-perf.config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		ReportMode string `json:"reportMode"`
		Profiles   map[string]struct {
			ReportMode string `json:"reportMode"`
			QPS        struct {
				MedicalModelQuery             float64 `json:"medicalQuery"`
				PersonalityModelQuery         float64 `json:"personalityQuery"`
				QuestionnaireQuery            float64 `json:"questionnaireQuery"`
				PersonalityQuestionnaireQuery float64 `json:"personalityQuestionnaireQuery"`
				Submit                        float64 `json:"submit"`
				Report                        float64 `json:"report"`
				Statistics                    float64 `json:"stats"`
				AsyncChainProbe               float64 `json:"chainProbe"`
			} `json:"qps"`
		} `json:"qpsProfiles"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if config.ReportMode != "websocket" {
		t.Fatalf("default reportMode = %q, want websocket", config.ReportMode)
	}

	assertProfile := func(name string, want struct {
		reportMode                    string
		medicalModelQuery             float64
		personalityModelQuery         float64
		questionnaireQuery            float64
		personalityQuestionnaireQuery float64
		submit                        float64
		report                        float64
		statistics                    float64
		asyncChainProbe               float64
	}) {
		t.Helper()
		got, ok := config.Profiles[name]
		if !ok {
			t.Fatalf("missing qps profile %q", name)
		}
		if got.ReportMode != want.reportMode ||
			got.QPS.MedicalModelQuery != want.medicalModelQuery ||
			got.QPS.PersonalityModelQuery != want.personalityModelQuery ||
			got.QPS.QuestionnaireQuery != want.questionnaireQuery ||
			got.QPS.PersonalityQuestionnaireQuery != want.personalityQuestionnaireQuery ||
			got.QPS.Submit != want.submit ||
			got.QPS.Report != want.report ||
			got.QPS.Statistics != want.statistics ||
			got.QPS.AsyncChainProbe != want.asyncChainProbe {
			t.Fatalf("profile %s mismatch: got %+v want %+v", name, got, want)
		}
	}

	assertProfile("mixed_300", struct {
		reportMode                    string
		medicalModelQuery             float64
		personalityModelQuery         float64
		questionnaireQuery            float64
		personalityQuestionnaireQuery float64
		submit                        float64
		report                        float64
		statistics                    float64
		asyncChainProbe               float64
	}{
		reportMode: "websocket", medicalModelQuery: 80, personalityModelQuery: 40,
		questionnaireQuery: 13, personalityQuestionnaireQuery: 13, submit: 24,
		report: 100, statistics: 29, asyncChainProbe: 1,
	})
	assertProfile("mixed_300_http_query", struct {
		reportMode                    string
		medicalModelQuery             float64
		personalityModelQuery         float64
		questionnaireQuery            float64
		personalityQuestionnaireQuery float64
		submit                        float64
		report                        float64
		statistics                    float64
		asyncChainProbe               float64
	}{
		reportMode: "websocket", medicalModelQuery: 80, personalityModelQuery: 40,
		questionnaireQuery: 13, personalityQuestionnaireQuery: 13, submit: 24,
		report: 96, statistics: 29, asyncChainProbe: 0,
	})

	sopRaw, err := os.ReadFile(filepath.Join(root, "docs/04-接口与运维/11-300QPS混合场景压测SOP.md"))
	if err != nil {
		t.Fatal(err)
	}
	sop := string(sopRaw)
	for _, want := range []string{
		"4C/8G**：`mixed_280_models` **边际通过",
		"`mixed_300_http_query` **通过",
		"`mixed_300` 全量 **未过",
		"8C/16G 全量已通过；4C/8G 未承诺",
	} {
		if !strings.Contains(sop, want) {
			t.Fatalf("SOP missing capacity contract fragment %q", want)
		}
	}
	if strings.Contains(sop, "`perf-mixed300`（**全量验收，已通过**）") {
		t.Fatal("SOP still describes perf-mixed300 as unqualified full-pass")
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
}
