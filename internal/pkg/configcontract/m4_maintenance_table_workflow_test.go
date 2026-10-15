package configcontract

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMaintenanceTableNativeWorkflowRequiresFreshExact65(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/workflows/ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Timeout int `yaml:"timeout-minutes"`
			Steps   []struct {
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if yaml.Unmarshal(raw, &workflow) != nil {
		t.Fatal("native workflow is invalid")
	}
	job := workflow.Jobs["metadata-readiness-mongodb-native"]
	runs := ""
	for _, step := range job.Steps {
		runs += step.Run
	}
	if job.Timeout != 5*8+5*3+20 {
		t.Fatal("five bounded phases need 40m tests,15m readiness sleeps,20m setup/proof/cleanup")
	}
	for _, required := range []string{
		`case "$1" in m|a|b|d|g)`,
		`start_owned_fixture d`, `start_owned_fixture g`,
		`export QS_M4_D_OBJECT_NATIVE_REQUIRED=1`,
		`export QS_M4_D_OBJECT_NATIVE_DISPOSABLE_ACK=owned_synthetic_dependencies_only`,
		`export QS_M4_D_OBJECT_NATIVE_MYSQL_DSN="root:m4-owned-object-fixture@tcp(127.0.0.1:${OBJECT_MYSQL_PORT})/?multiStatements=true"`,
		`-ldflags="-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$object_source"`,
		`-run='^TestM4SQLObjectNative'`, `-run='^TestM4MongoObjectNative'`,
		`make test-metadata-readiness-mongodb-native`,
	} {
		if !strings.Contains(runs, required) {
			t.Fatal("D native phase lost its real selector or owned dependency boundary", required)
		}
	}
	bEnd := strings.LastIndex(runs, "PY_MONGO_OBJECT_PROOF")
	dStart := strings.Index(runs, "start_owned_fixture d\n")
	gStart := strings.Index(runs, "start_owned_fixture g\n")
	originalProof := strings.Index(runs, `d_native_proof original maintenance-object-native.jsonl "$object_source"`)
	generationProof := strings.Index(runs, `d_native_proof generation runtime-generation-native.jsonl "$object_source"`)
	unionProof := strings.Index(runs, `d_native_proof union maintenance-object-native.jsonl runtime-generation-native.jsonl "$object_source"`)
	if bEnd < 0 || dStart <= bEnd || originalProof <= dStart || gStart <= originalProof || generationProof <= gStart || unionProof <= generationProof {
		t.Fatal("D requires original53 then generation12 then their same-source union")
	}
	for _, boundary := range [][2]int{{bEnd, dStart}, {originalProof, gStart}, {generationProof, unionProof}} {
		if strings.Count(runs[boundary[0]:boundary[1]], "cleanup_owned_fixture\n") != 1 {
			t.Fatal("fresh phase cannot begin or certify union before prior healthy cleanup/absence")
		}
	}
	for _, boundary := range [][2]int{{originalProof, gStart}, {generationProof, unionProof}} {
		cleanup := strings.Index(runs[boundary[0]:boundary[1]], "cleanup_owned_fixture\n")
		reset := strings.Index(runs[boundary[0]:boundary[1]], "unset TMPDIR\n")
		if reset <= cleanup {
			t.Fatal("released private TMPDIR must not leak into fresh dependency startup or union")
		}
	}
	for _, required := range []string{
		`export QS_M4_D_OBJECT_NATIVE_MONGO_URI="$QS_M4_OBJECT_NATIVE_MONGO_URI"`,
		`d_native_tmp="$(mktemp -d "$HOME/qs-m4-d-native-${fixture_run_owner}.XXXXXX")"`,
		`export TMPDIR="$d_native_tmp"`,
	} {
		if strings.Count(runs[dStart:unionProof], required) != 2 || !strings.Contains(runs[gStart:generationProof], required) {
			t.Fatal("each D phase must bind its fresh URI,connection and private material root", required)
		}
	}
	marker := "<<'PY_D_OBJECT_PROOF'\n"
	start := strings.Index(runs, marker)
	if start < 0 {
		t.Fatal("D proof has no executable body")
	}
	proof := runs[start+len(marker):]
	end := strings.Index(proof, "\nPY_D_OBJECT_PROOF")
	if end < 0 {
		t.Fatal("D proof terminator missing")
	}
	proof = proof[:end]
	namePattern := regexp.MustCompile(`(?m)^\s+'(TestM4MaintenanceTablesNative[^']*)',?$`)
	phaseNames := func(variable string, want int) []string {
		t.Helper()
		_, body, ok := strings.Cut(proof, variable+" = {\n")
		if !ok {
			t.Fatal("D proof lacks its fixed phase selectors", variable)
		}
		body, _, ok = strings.Cut(body, "\n}")
		if !ok {
			t.Fatal("D proof phase selectors not closed")
		}
		matches := namePattern.FindAllStringSubmatch(body, -1)
		if len(matches) != want {
			t.Fatal("D proof must retain its exact original53 and generation12 lists")
		}
		names := make([]string, 0, want)
		for _, match := range matches {
			names = append(names, match[1])
		}
		return names
	}
	originalNames, generationNames := phaseNames("original_expected", 53), phaseNames("generation_expected", 12)
	names := append(append([]string{}, originalNames...), generationNames...)
	tops := map[string]bool{}
	files, err := filepath.Glob(filepath.Join(repoRoot(t), "scripts/oneoff/m4_maintenance_table_protection/native*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && strings.HasPrefix(function.Name.Name, "TestM4MaintenanceTablesNative") {
				tops[function.Name.Name] = true
			}
		}
	}
	expected := map[string]bool{}
	for _, name := range names {
		if expected[name] || !tops[strings.Split(name, "/")[0]] {
			t.Fatal("D proof has duplicate or nonexistent parent selectors")
		}
		expected[name] = true
	}
	if !expected["TestM4MaintenanceTablesNativeFirstRuntimeGate"] || !expected["TestM4MaintenanceTablesNativeRuntimeGeneration"] || len(tops) != 17 {
		t.Fatal("D proof must retain all original sixteen and new generation top-level tests")
	}
	selectors := regexp.MustCompile(`-run='([^']+)' ./scripts/oneoff/m4_maintenance_table_protection`).FindAllStringSubmatch(runs, -1)
	if len(selectors) != 2 {
		t.Fatal("D must run exactly two bounded selectors")
	}
	originalSelector, err := regexp.Compile(selectors[0][1])
	if err != nil {
		t.Fatal(err)
	}
	generationSelector, err := regexp.Compile(selectors[1][1])
	if err != nil || selectors[1][1] != "^TestM4MaintenanceTablesNativeRuntimeGeneration$" || !strings.HasPrefix(selectors[0][1], "^(") || !strings.HasSuffix(selectors[0][1], ")$") {
		t.Fatal("D phase selectors must be exactly anchored")
	}
	for top := range tops {
		if originalSelector.MatchString(top) != (top != "TestM4MaintenanceTablesNativeRuntimeGeneration") || generationSelector.MatchString(top) != (top == "TestM4MaintenanceTablesNativeRuntimeGeneration") || originalSelector.MatchString(top+"Unexpected") {
			t.Fatal("D phase selectors overlap,omit or overmatch an actual top-level test", top)
		}
	}
	if originalSelector.MatchString("TestM4MaintenanceTablesNativeUnknown") {
		t.Fatal("original D phase accepts a future unreviewed test")
	}
	source := strings.Repeat("a", 40)
	eventsFor := func(names []string) []map[string]any {
		events := make([]map[string]any, 0, len(names)*2+2)
		for _, name := range names {
			events = append(events, map[string]any{"Action": "run", "Test": name}, map[string]any{"Action": "pass", "Test": name})
		}
		return append(events,
			map[string]any{"Action": "output", "Test": names[0], "Output": "actual compiled D fixture tool=" + source + " phase=before_dependency_binding\n"},
			map[string]any{"Action": "output", "Test": names[0], "Output": "actual owned dependencies mysql=8.0.36 mongo=7.0.31 rs0; Dsource=c335b2a1a34dd69830261e2d8e11b16da3f8befb resources=20e0096ffa50a50a7818bfd09741fa48ce8d6e0dc909bf0cf49e0292ba254c4d files=286; tool=" + source + "\n"},
		)
	}
	originalEvents, generationEvents := eventsFor(originalNames), eventsFor(generationNames)
	writeEvents := func(t *testing.T, events []map[string]any) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "proof.jsonl")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if json.NewEncoder(file).Encode(event) != nil {
				t.Fatal("could not save local proof input")
			}
		}
		if file.Close() != nil {
			t.Fatal("local proof input release failed")
		}
		return path
	}
	for _, name := range []string{"complete", "missing_run", "missing_pass", "duplicate", "unknown", "skip", "fail", "wrong_source", "missing_entry", "original_only", "generation_only", "swapped_logs", "old_full65", "generation_in_original", "wrong_generation_source", "missing_generation_entry", "duplicate_generation", "missing_generation_pass"} {
		t.Run(name, func(t *testing.T) {
			original, generation := append([]map[string]any{}, originalEvents...), append([]map[string]any{}, generationEvents...)
			mode := "union"
			switch name {
			case "missing_run":
				original = original[1:]
			case "missing_pass":
				original = append(original[:1], original[2:]...)
			case "duplicate":
				original = append(original, originalEvents[0])
			case "unknown":
				original = append(original, map[string]any{"Action": "run", "Test": "TestM4MaintenanceTablesNativeUnknown"})
			case "skip", "fail":
				original = append(original, map[string]any{"Action": name, "Test": originalNames[0]})
			case "wrong_source", "wrong_generation_source":
				changed := original
				if name == "wrong_generation_source" {
					changed = generation
				}
				changed[len(changed)-1] = map[string]any{"Action": "output", "Test": changed[0]["Test"], "Output": strings.ReplaceAll(changed[len(changed)-1]["Output"].(string), source, strings.Repeat("b", 40))}
			case "missing_entry":
				original = append(original[:len(original)-2], original[len(original)-1])
			case "original_only":
				mode = "original"
			case "generation_only":
				mode = "generation"
			case "swapped_logs":
				original, generation = generation, original
			case "old_full65":
				original = append(original, generation...)
			case "generation_in_original":
				original = append(original, generation[0])
			case "missing_generation_entry":
				generation = append(generation[:len(generation)-2], generation[len(generation)-1])
			case "duplicate_generation":
				generation = append(generation, generationEvents[0])
			case "missing_generation_pass":
				generation = append(generation[:1], generation[2:]...)
			}
			originalPath, generationPath := writeEvents(t, original), writeEvents(t, generation)
			args := []string{"-c", proof, mode, originalPath, generationPath, source}
			switch mode {
			case "original":
				args = []string{"-c", proof, mode, originalPath, source}
			case "generation":
				args = []string{"-c", proof, mode, generationPath, source}
			}
			output, err := exec.Command("python3", args...).CombinedOutput()
			wantPass := name == "complete" || name == "original_only" || name == "generation_only"
			if wantPass != (err == nil) {
				t.Fatalf("native proof did not enforce its phase/union evidence: %v %s", err, output)
			}
		})
	}
}
