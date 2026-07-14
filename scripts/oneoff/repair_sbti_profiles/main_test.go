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

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	rulesetinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestRepairDefinitionBackfillsCanonicalProfilesAndIsIdempotent(t *testing.T) {
	broken, catalog := brokenSBTIDefinition(t)

	repaired, summary, err := repairDefinition(broken, catalog)
	if err != nil {
		t.Fatalf("repairDefinition: %v", err)
	}
	if summary.ProfileCount != 27 || summary.NormalCount != 25 || summary.SpecialCount != 2 {
		t.Fatalf("unexpected profile counts: %+v", summary)
	}
	if summary.PatternChanges != 25 || summary.SpecialFlagChanges != 2 || summary.TriggerChanges != 2 || summary.AdapterChanges != 2 || len(summary.Changes) != 31 {
		t.Fatalf("unexpected changes: %+v", summary)
	}
	if err := verifyRepairedDefinition(repaired, catalog); err != nil {
		t.Fatalf("verify repaired definition: %v", err)
	}

	second, secondSummary, err := repairDefinition(repaired, catalog)
	if err != nil {
		t.Fatalf("second repairDefinition: %v", err)
	}
	if secondSummary.Changed() {
		t.Fatalf("second repair should be a no-op: %+v", secondSummary)
	}
	assertJSONEqual(t, repaired, second)
}

func TestRepairDefinitionResumesAfterProfilesWereAlreadySaved(t *testing.T) {
	broken, catalog := brokenSBTIDefinition(t)
	profileRepaired, _, err := repairDefinition(broken, catalog)
	if err != nil {
		t.Fatalf("initial repairDefinition: %v", err)
	}
	partial := setSBTIAdapters(t, profileRepaired, "sbti")

	repaired, summary, err := repairDefinition(partial, catalog)
	if err != nil {
		t.Fatalf("resume repairDefinition: %v", err)
	}
	if summary.PatternChanges != 0 || summary.SpecialFlagChanges != 0 || summary.TriggerChanges != 0 || summary.AdapterChanges != 2 || len(summary.Changes) != 2 {
		t.Fatalf("resume summary = %+v, want only two adapter changes", summary)
	}
	if err := verifyRepairedDefinition(repaired, catalog); err != nil {
		t.Fatalf("verify resumed repair: %v", err)
	}
}

func TestRepairDefinitionRejectsUnrelatedAdapter(t *testing.T) {
	broken, catalog := brokenSBTIDefinition(t)
	input := setSBTIAdapters(t, broken, "trait_profile")
	_, _, err := repairDefinition(input, catalog)
	if err == nil || !strings.Contains(err.Error(), "refusing to replace an unrelated adapter") {
		t.Fatalf("repairDefinition() error = %v, want unrelated adapter rejection", err)
	}
}

func TestWikiRepairCatalogMatchesLegacySeedExecutionProfiles(t *testing.T) {
	catalog, err := loadWikiRepairCatalog()
	if err != nil {
		t.Fatalf("loadWikiRepairCatalog: %v", err)
	}
	if catalog.Source != sbtiWikiRepository || catalog.Revision != sbtiWikiRevision || catalog.License != sbtiWikiLicense {
		t.Fatalf("unexpected provenance: %+v", catalog)
	}
	seed, err := rulesetinfra.LoadDefaultSBTILegacyModel()
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(catalog.DimensionOrder) != fmt.Sprint(seed.DimensionOrder) {
		t.Fatalf("dimension order drifted: wiki=%v seed=%v", catalog.DimensionOrder, seed.DimensionOrder)
	}
	if len(catalog.Profiles) != 27 {
		t.Fatalf("wiki profile count = %d, want 27", len(catalog.Profiles))
	}
	for code, want := range catalog.Profiles {
		got, ok := legacySeedProfile(seed, code)
		if !ok || got != want {
			t.Fatalf("profile %s drifted: wiki=%+v seed=%+v present=%t", code, want, got, ok)
		}
	}
}

func legacySeedProfile(seed *modeltypology.SBTILegacyModel, code string) (profileSeed, bool) {
	for _, outcome := range seed.NormalOutcomes {
		if outcome.Code == code {
			return profileSeed{Pattern: outcome.Pattern, IsSpecial: outcome.IsSpecial}, true
		}
	}
	for _, outcome := range seed.SpecialOutcomes {
		if outcome.Code == code {
			return profileSeed{Pattern: outcome.Pattern, Trigger: outcome.Trigger, IsSpecial: outcome.IsSpecial}, true
		}
	}
	return profileSeed{}, false
}

func TestRepairDefinitionPreservesUnknownFields(t *testing.T) {
	broken, catalog := brokenSBTIDefinition(t)
	var root map[string]json.RawMessage
	if err := json.Unmarshal(broken, &root); err != nil {
		t.Fatal(err)
	}
	root["FutureTopLevel"] = mustJSON(map[string]any{"keep": true})
	var conclusions []map[string]json.RawMessage
	if err := json.Unmarshal(root["Conclusions"], &conclusions); err != nil {
		t.Fatal(err)
	}
	var profiles []map[string]json.RawMessage
	if err := json.Unmarshal(conclusions[0]["Profiles"], &profiles); err != nil {
		t.Fatal(err)
	}
	profiles[0]["FutureProfileField"] = mustJSON(map[string]any{"keep": "yes"})
	conclusions[0]["Profiles"] = mustJSON(profiles)
	root["Conclusions"] = mustJSON(conclusions)
	input := mustJSON(root)

	repaired, _, err := repairDefinition(input, catalog)
	if err != nil {
		t.Fatalf("repairDefinition: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(repaired, &got); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, []byte(`{"keep":true}`), got["FutureTopLevel"])
	if err := json.Unmarshal(got["Conclusions"], &conclusions); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(conclusions[0]["Profiles"], &profiles); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, []byte(`{"keep":"yes"}`), profiles[0]["FutureProfileField"])
}

func TestRepairDefinitionRejectsMismatchedFactorOrderAndOutcomeCodes(t *testing.T) {
	broken, catalog := brokenSBTIDefinition(t)
	t.Run("factor order", func(t *testing.T) {
		var root map[string]json.RawMessage
		if err := json.Unmarshal(broken, &root); err != nil {
			t.Fatal(err)
		}
		var measure map[string]json.RawMessage
		if err := json.Unmarshal(root["Measure"], &measure); err != nil {
			t.Fatal(err)
		}
		var graph map[string]json.RawMessage
		if err := json.Unmarshal(measure["FactorGraph"], &graph); err != nil {
			t.Fatal(err)
		}
		var roots []string
		if err := json.Unmarshal(graph["Roots"], &roots); err != nil {
			t.Fatal(err)
		}
		roots[0], roots[1] = roots[1], roots[0]
		graph["Roots"] = mustJSON(roots)
		measure["FactorGraph"] = mustJSON(graph)
		root["Measure"] = mustJSON(measure)
		_, _, err := repairDefinition(mustJSON(root), catalog)
		if err == nil || !strings.Contains(err.Error(), "canonical SBTI order") {
			t.Fatalf("expected factor-order rejection, got %v", err)
		}
	})

	t.Run("outcome codes", func(t *testing.T) {
		var root map[string]json.RawMessage
		if err := json.Unmarshal(broken, &root); err != nil {
			t.Fatal(err)
		}
		var outcomes []json.RawMessage
		if err := json.Unmarshal(root["Outcomes"], &outcomes); err != nil {
			t.Fatal(err)
		}
		root["Outcomes"] = mustJSON(outcomes[:len(outcomes)-1])
		_, _, err := repairDefinition(mustJSON(root), catalog)
		if err == nil || !strings.Contains(err.Error(), "outcome codes do not match") {
			t.Fatalf("expected outcome-code rejection, got %v", err)
		}
	})
}

func TestRunApplyWritesBackupUsesProtectedAPIAndDoesNotPublish(t *testing.T) {
	broken, catalog := brokenSBTIDefinition(t)
	var mu sync.Mutex
	methods := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/assessment-models/SBTI_FUN/definition":
			writeEnvelope(t, w, broken)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/assessment-models/SBTI_FUN/definition":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if err := verifyRepairedDefinition(body, catalog); err != nil {
				t.Errorf("PUT body not repaired: %v", err)
			}
			writeEnvelope(t, w, body)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/assessment-models/SBTI_FUN/validate":
			writeEnvelope(t, w, mustJSON(map[string]any{"passed": true, "valid": true, "issues": []any{}, "errors": []any{}}))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	backupDir := t.TempDir()
	var out strings.Builder
	err := run(context.Background(), config{
		APIBase:   server.URL + "/api/v1",
		ModelCode: "SBTI_FUN",
		Token:     "test-token",
		BackupDir: backupDir,
		Apply:     true,
		Timeout:   5 * time.Second,
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
	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"GET /api/v1/assessment-models/SBTI_FUN/definition",
		"PUT /api/v1/assessment-models/SBTI_FUN/definition",
		"POST /api/v1/assessment-models/SBTI_FUN/validate",
	}
	if fmt.Sprint(methods) != fmt.Sprint(want) {
		t.Fatalf("requests = %v, want %v", methods, want)
	}
	if strings.Contains(strings.Join(methods, "\n"), "publish") {
		t.Fatalf("repair must not publish: %v", methods)
	}
}

func TestRunDryRunOnlyReadsDefinition(t *testing.T) {
	broken, _ := brokenSBTIDefinition(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet {
			t.Errorf("dry-run request method = %s, want GET", r.Method)
		}
		writeEnvelope(t, w, broken)
	}))
	defer server.Close()

	var out strings.Builder
	err := run(context.Background(), config{
		APIBase:   server.URL,
		ModelCode: "SBTI_FUN",
		Token:     "test-token",
		BackupDir: t.TempDir(),
		Timeout:   5 * time.Second,
	}, &out)
	if err != nil {
		t.Fatalf("run dry-run: %v", err)
	}
	if requests != 1 || !strings.Contains(out.String(), "Dry run complete") {
		t.Fatalf("unexpected dry-run result: requests=%d output=%s", requests, out.String())
	}
}

func brokenSBTIDefinition(t *testing.T) ([]byte, repairCatalog) {
	t.Helper()
	seed, err := rulesetinfra.LoadDefaultSBTILegacyModel()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := loadWikiRepairCatalog()
	if err != nil {
		t.Fatal(err)
	}
	payload := modeltypology.FromSBTI(seed)
	if payload == nil {
		t.Fatal("FromSBTI returned nil")
	}
	runtime, err := payload.ToRuntimeSpec()
	if err != nil {
		t.Fatal(err)
	}
	definition := modeltypology.DefinitionFromRuntime(payload, runtime)
	for index, item := range definition.Conclusions {
		typed, ok := item.(conclusion.TypeConclusion)
		if !ok {
			continue
		}
		for profileIndex := range typed.Profiles {
			typed.Profiles[profileIndex].Pattern = ""
			typed.Profiles[profileIndex].IsSpecial = false
			typed.Profiles[profileIndex].Trigger = ""
		}
		typed.OutcomeMapping.DetailAdapterKey = "sbti"
		definition.Conclusions[index] = typed
	}
	if len(definition.ReportMap.Sections) == 0 {
		t.Fatal("SBTI definition report map is empty")
	}
	definition.ReportMap.Sections[0].AdapterKey = "sbti"
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	return raw, catalog
}

func setSBTIAdapters(t *testing.T, input []byte, adapter string) []byte {
	t.Helper()
	var definition map[string]json.RawMessage
	if err := json.Unmarshal(input, &definition); err != nil {
		t.Fatal(err)
	}
	var conclusions []map[string]json.RawMessage
	if err := json.Unmarshal(definition["Conclusions"], &conclusions); err != nil {
		t.Fatal(err)
	}
	for index := range conclusions {
		var kind string
		if err := json.Unmarshal(conclusions[index]["Kind"], &kind); err != nil {
			t.Fatal(err)
		}
		if kind != "type" {
			continue
		}
		var mapping map[string]json.RawMessage
		if err := json.Unmarshal(conclusions[index]["OutcomeMapping"], &mapping); err != nil {
			t.Fatal(err)
		}
		mapping["DetailAdapterKey"] = mustJSON(adapter)
		conclusions[index]["OutcomeMapping"] = mustJSON(mapping)
	}
	definition["Conclusions"] = mustJSON(conclusions)
	var reportMap map[string]json.RawMessage
	if err := json.Unmarshal(definition["ReportMap"], &reportMap); err != nil {
		t.Fatal(err)
	}
	var sections []map[string]json.RawMessage
	if err := json.Unmarshal(reportMap["Sections"], &sections); err != nil {
		t.Fatal(err)
	}
	if len(sections) == 0 {
		t.Fatal("report sections are empty")
	}
	sections[0]["AdapterKey"] = mustJSON(adapter)
	reportMap["Sections"] = mustJSON(sections)
	definition["ReportMap"] = mustJSON(reportMap)
	return mustJSON(definition)
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, data []byte) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := fmt.Fprintf(w, `{"code":0,"message":"success","data":%s}`, data); err != nil {
		t.Error(err)
	}
}

func assertJSONEqual(t *testing.T, want, got []byte) {
	t.Helper()
	var wantValue any
	var gotValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("decode want JSON: %v", err)
	}
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decode got JSON: %v", err)
	}
	wantJSON, _ := json.Marshal(wantValue)
	gotJSON, _ := json.Marshal(gotValue)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("JSON differs\nwant: %s\n got: %s", wantJSON, gotJSON)
	}
}
