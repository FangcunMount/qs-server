package configcontract

import (
	"encoding/json"
	"os"
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
