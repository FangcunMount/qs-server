package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDefaultTargetsArePinnedToTheFourFailedModelsOnly(t *testing.T) {
	want := []migrationTarget{
		{Code: "MBTI_OEJTS", ExpectedVersion: "v25", LegacyAdapter: "mbti", GenericAdapter: "personality_type"},
		{Code: "MBTI_FC_93", ExpectedVersion: "v15", LegacyAdapter: "mbti", GenericAdapter: "personality_type"},
		{Code: "SBTI_FUN", ExpectedVersion: "v29", LegacyAdapter: "sbti", GenericAdapter: "personality_type"},
		{Code: "BIG5_IPIP_50", ExpectedVersion: "v9", LegacyAdapter: "bigfive", GenericAdapter: "trait_profile"},
	}
	if fmt.Sprintf("%#v", defaultTargets) != fmt.Sprintf("%#v", want) {
		t.Fatalf("default targets = %#v, want %#v", defaultTargets, want)
	}
}

func TestNormalizeDefinitionNormalizesOnlyAdaptersAndIsIdempotent(t *testing.T) {
	target := migrationTarget{Code: "MBTI_OEJTS", ExpectedVersion: "v25", LegacyAdapter: "mbti", GenericAdapter: "personality_type"}
	broken := definitionFixture(t, target, "keep-me")

	normalized, summary, err := normalizeDefinition(broken, target)
	if err != nil {
		t.Fatalf("normalizeDefinition: %v", err)
	}
	if summary.OutcomeAdapter != "personality_type" || summary.ReportAdapter != "personality_type" {
		t.Fatalf("summary = %+v, want both personality_type", summary)
	}
	if err := verifyNormalizedDefinition(normalized, target); err != nil {
		t.Fatalf("verifyNormalizedDefinition: %v", err)
	}
	assertPreservedMarker(t, normalized, "keep-me")

	second, secondSummary, err := normalizeDefinition(normalized, target)
	if err != nil {
		t.Fatalf("second normalizeDefinition: %v", err)
	}
	if secondSummary.Changed() {
		t.Fatalf("second normalization should be no-op: %+v", secondSummary)
	}
	assertJSONEqual(t, normalized, second)
}

func TestNormalizeDefinitionSupportsBigFiveGenericTraitProfile(t *testing.T) {
	target := migrationTarget{Code: "BIG5_IPIP_50", ExpectedVersion: "v9", LegacyAdapter: "bigfive", GenericAdapter: "trait_profile"}
	normalized, summary, err := normalizeDefinition(definitionFixture(t, target, "big-five"), target)
	if err != nil {
		t.Fatalf("normalizeDefinition: %v", err)
	}
	if !summary.Changed() {
		t.Fatal("normalization should change both legacy bigfive adapters")
	}
	if err := verifyNormalizedDefinition(normalized, target); err != nil {
		t.Fatalf("verifyNormalizedDefinition: %v", err)
	}
}

func TestNormalizeDefinitionRejectsUnexpectedAdapter(t *testing.T) {
	target := migrationTarget{Code: "SBTI_FUN", ExpectedVersion: "v29", LegacyAdapter: "sbti", GenericAdapter: "personality_type"}
	broken := definitionFixture(t, target, "sbti")
	var root map[string]any
	if err := json.Unmarshal(broken, &root); err != nil {
		t.Fatal(err)
	}
	conclusion := root["Conclusions"].([]any)[0].(map[string]any)
	conclusion["OutcomeMapping"].(map[string]any)["DetailAdapterKey"] = "mbti"
	_, _, err := normalizeDefinition(mustJSON(root), target)
	if err == nil || !strings.Contains(err.Error(), "refusing to replace") {
		t.Fatalf("expected strict adapter refusal, got %v", err)
	}
}

func TestRunApplyAndPublishUsesProtectedAPIsAndVerifiesNewSnapshot(t *testing.T) {
	target := migrationTarget{Code: "MBTI_FC_93", ExpectedVersion: "v15", LegacyAdapter: "mbti", GenericAdapter: "personality_type"}
	broken := definitionFixture(t, target, "published-before")
	var mu sync.Mutex
	currentDefinition := broken
	currentVersion := target.ExpectedVersion
	currentStatus := "published"
	requests := make([]string, 0, 6)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/assessment-models/MBTI_FC_93":
			writeEnvelope(t, w, map[string]any{"code": target.Code, "status": currentStatus})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/assessment-models/published/MBTI_FC_93":
			writeEnvelope(t, w, map[string]any{"code": target.Code, "status": currentStatus, "version": currentVersion, "definition": json.RawMessage(currentDefinition)})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/assessment-models/MBTI_FC_93/definition":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if err := verifyNormalizedDefinition(body, target); err != nil {
				t.Errorf("PUT body: %v", err)
			}
			currentDefinition = body
			currentStatus = "draft"
			writeEnvelope(t, w, json.RawMessage(body))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/assessment-models/MBTI_FC_93/validate":
			writeEnvelope(t, w, map[string]any{"passed": true, "issues": []any{}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/assessment-releases/MBTI_FC_93/publish":
			currentStatus = "published"
			// The published snapshot version is derived from the model's current
			// configuration revision, which may be unrelated to the old snapshot
			// version. This mirrors a legacy snapshot v15 being republished as v40.
			currentVersion = "v40"
			writeEnvelope(t, w, map[string]any{"model_code": target.Code, "model_status": currentStatus})
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	backupDir := t.TempDir()
	var out strings.Builder
	err := run(context.Background(), config{
		APIBase:   server.URL + "/api/v1",
		Token:     "test-token",
		BackupDir: backupDir,
		Apply:     true,
		Publish:   true,
		Timeout:   5 * time.Second,
		Targets:   []migrationTarget{target},
	}, &out)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("backup files = %d, want 1", len(entries))
	}
	backup, err := os.ReadFile(filepath.Join(backupDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, broken, backup)
	want := []string{
		"GET /api/v1/assessment-models/MBTI_FC_93",
		"GET /api/v1/assessment-models/published/MBTI_FC_93",
		"PUT /api/v1/assessment-models/MBTI_FC_93/definition",
		"POST /api/v1/assessment-models/MBTI_FC_93/validate",
		"POST /api/v1/assessment-releases/MBTI_FC_93/publish",
		"GET /api/v1/assessment-models/published/MBTI_FC_93",
	}
	if fmt.Sprint(requests) != fmt.Sprint(want) {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
}

func TestRunSkipsPreviouslyNormalizedTargetAfterVersionChanged(t *testing.T) {
	target := migrationTarget{Code: "MBTI_OEJTS", ExpectedVersion: "v25", LegacyAdapter: "mbti", GenericAdapter: "personality_type"}
	normalized, _, err := normalizeDefinition(definitionFixture(t, target, "already-migrated"), target)
	if err != nil {
		t.Fatalf("normalize fixture: %v", err)
	}
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/assessment-models/MBTI_OEJTS":
			writeEnvelope(t, w, map[string]any{"code": target.Code, "status": "published"})
		case "/api/v1/assessment-models/published/MBTI_OEJTS":
			writeEnvelope(t, w, map[string]any{"code": target.Code, "status": "published", "version": "v40", "definition": json.RawMessage(normalized)})
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	err = run(context.Background(), config{APIBase: server.URL + "/api/v1", Token: "test-token", Apply: true, Publish: true, BackupDir: t.TempDir(), Timeout: 5 * time.Second, Targets: []migrationTarget{target}}, io.Discard)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := []string{"GET /api/v1/assessment-models/MBTI_OEJTS", "GET /api/v1/assessment-models/published/MBTI_OEJTS"}
	if fmt.Sprint(requests) != fmt.Sprint(want) {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
}

func TestRunDryRunDoesNotWrite(t *testing.T) {
	target := migrationTarget{Code: "SBTI_FUN", ExpectedVersion: "v29", LegacyAdapter: "sbti", GenericAdapter: "personality_type"}
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/assessment-models/SBTI_FUN":
			writeEnvelope(t, w, map[string]any{"code": target.Code, "status": "published"})
		case "/api/v1/assessment-models/published/SBTI_FUN":
			writeEnvelope(t, w, map[string]any{"code": target.Code, "status": "published", "version": target.ExpectedVersion, "definition": json.RawMessage(definitionFixture(t, target, "dry-run"))})
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	err := run(context.Background(), config{APIBase: server.URL + "/api/v1", Token: "test-token", Timeout: 5 * time.Second, Targets: []migrationTarget{target}}, io.Discard)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := []string{"GET /api/v1/assessment-models/SBTI_FUN", "GET /api/v1/assessment-models/published/SBTI_FUN"}
	if fmt.Sprint(requests) != fmt.Sprint(want) {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
}

func TestRunRefusesVersionDriftBeforeWriting(t *testing.T) {
	target := migrationTarget{Code: "MBTI_OEJTS", ExpectedVersion: "v25", LegacyAdapter: "mbti", GenericAdapter: "personality_type"}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/api/v1/assessment-models/MBTI_OEJTS":
			writeEnvelope(t, w, map[string]any{"code": target.Code, "status": "published"})
		case "/api/v1/assessment-models/published/MBTI_OEJTS":
			writeEnvelope(t, w, map[string]any{"code": target.Code, "status": "published", "version": "v26", "definition": json.RawMessage(definitionFixture(t, target, "drift"))})
		default:
			http.Error(w, "unexpected write", http.StatusNotFound)
		}
	}))
	defer server.Close()

	err := run(context.Background(), config{APIBase: server.URL + "/api/v1", Token: "test-token", Apply: true, BackupDir: t.TempDir(), Timeout: 5 * time.Second, Targets: []migrationTarget{target}}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "published version") {
		t.Fatalf("expected version-drift refusal, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2 reads only", requests)
	}
}

func TestParseConfigRejectsPublishWithoutApply(t *testing.T) {
	_, err := parseConfig([]string{"--api-base", "https://qs.example.com", "--publish"}, io.Discard, func(key string) string {
		if key == "QS_OPERATOR_TOKEN" {
			return "token"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "requires --apply") {
		t.Fatalf("expected --publish guard, got %v", err)
	}
}

func definitionFixture(t *testing.T, target migrationTarget, marker string) []byte {
	t.Helper()
	return mustJSON(map[string]any{
		"Measure": map[string]any{"Marker": marker},
		"Conclusions": []any{map[string]any{
			"Kind": "type",
			"OutcomeMapping": map[string]any{
				"DetailKind":       target.GenericAdapter,
				"DetailAdapterKey": target.LegacyAdapter,
				"Algorithm":        "factor_classification",
			},
			"Profiles": []any{},
		}},
		"Outcomes": []any{},
		"ReportMap": map[string]any{"Sections": []any{map[string]any{
			"Code":          "result",
			"Kind":          target.GenericAdapter,
			"AdapterKey":    target.LegacyAdapter,
			"TemplateID":    "do-not-change",
			"CategoryLabel": marker,
		}}},
	})
}

func assertPreservedMarker(t *testing.T, input []byte, want string) {
	t.Helper()
	var root struct {
		Measure   map[string]any `json:"Measure"`
		ReportMap struct {
			Sections []struct {
				TemplateID    string `json:"TemplateID"`
				CategoryLabel string `json:"CategoryLabel"`
			} `json:"Sections"`
		} `json:"ReportMap"`
	}
	if err := json.Unmarshal(input, &root); err != nil {
		t.Fatal(err)
	}
	if root.Measure["Marker"] != want || len(root.ReportMap.Sections) != 1 || root.ReportMap.Sections[0].TemplateID != "do-not-change" || root.ReportMap.Sections[0].CategoryLabel != want {
		t.Fatalf("unrelated fields changed: %+v", root)
	}
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data}); err != nil {
		t.Fatal(err)
	}
}

func assertJSONEqual(t *testing.T, left, right []byte) {
	t.Helper()
	var leftValue any
	var rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%#v", leftValue) != fmt.Sprintf("%#v", rightValue) {
		t.Fatalf("JSON differs:\nleft=%s\nright=%s", left, right)
	}
}
