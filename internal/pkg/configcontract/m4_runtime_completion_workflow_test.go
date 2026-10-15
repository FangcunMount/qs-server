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
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const committedRuntimeCompletionSelector = "^TestM4MaintenanceTablesCommittedRuntimeCompletionNative$"

func completionWorkflow(t *testing.T) (string, int) {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join(repoRoot(t), ".github/workflows/ci.yml"))
	if e != nil {
		t.Fatal(e)
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
		t.Fatal("workflow invalid")
	}
	job := workflow.Jobs["metadata-readiness-mongodb-native"]
	var run string
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "start_owned_fixture m\n") {
			run = step.Run
		}
	}
	if run == "" {
		t.Fatal("owned native fixture step missing")
	}
	return run, job.Timeout
}

func completionProof(t *testing.T) (string, []string) {
	t.Helper()
	run, _ := completionWorkflow(t)
	_, proof, ok := strings.Cut(run, "<<'PY_D_RUNTIME_COMPLETION_PROOF'\n")
	if !ok {
		t.Fatal("completion13 has no independent executable proof")
	}
	proof, _, ok = strings.Cut(proof, "\nPY_D_RUNTIME_COMPLETION_PROOF")
	if !ok {
		t.Fatal("completion13 proof not closed")
	}
	names := regexp.MustCompile(`(?m)^    '(TestM4MaintenanceTablesCommittedRuntimeCompletionNative[^']*)',$`).FindAllStringSubmatch(proof, -1)
	if len(names) != 13 {
		t.Fatal("completion proof must name top plus12 children")
	}
	required := make([]string, 0, 13)
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name[1]] {
			t.Fatal("duplicate completion requirement")
		}
		required = append(required, name[1])
		seen[name[1]] = true
	}
	return proof, required
}

func TestCommittedRuntimeCompletionNativeWorkflowHasSeparateFreshPhase(t *testing.T) {
	run, timeout := completionWorkflow(t)
	if timeout != ((8*8+8*3+20+9)/10)*10 {
		t.Fatal("eight bounded phases require64m tests+24m readiness+20m setup/proof/cleanup; ceil110")
	}
	phases := regexp.MustCompile(`(?m)^start_owned_fixture ([mabcdghi])$`).FindAllStringSubmatch(run, -1)
	if len(phases) != 8 {
		t.Fatal("metadata,A47,B37,C30,D53,D21,completion13,C-generation14 need eight fresh phases")
	}
	for i, phase := range []string{"m", "a", "b", "c", "d", "g", "h", "i"} {
		if phases[i][1] != phase {
			t.Fatal("existing phases lost or reordered")
		}
	}
	if strings.Count(run, "-timeout=8m -json") != 7 {
		t.Fatal("seven object commands plus metadata keep eight8m budgets")
	}
	union := strings.Index(run, `d_native_proof union maintenance-object-native.jsonl runtime-generation-native.jsonl "$object_source"`)
	start := strings.Index(run, "start_owned_fixture h\n")
	if union < 0 || start <= union {
		t.Fatal("completion must follow the unchanged standalone74 union")
	}
	next := strings.Index(run, "start_owned_fixture i\n")
	if next <= start {
		t.Fatal("C runtime generation must follow the completion phase")
	}
	tail := run[start:next]
	reset := strings.Index(tail, "\nunset TMPDIR\n")
	if reset < 0 {
		t.Fatal("completion phase must reset its own TMPDIR before the next fresh phase")
	}
	tail = tail[:reset+len("\nunset TMPDIR\n")]
	for _, required := range []string{`case "$1" in m|a|b|c|d|g|h|i)`, `export QS_M4_D_OBJECT_NATIVE_MONGO_URI="$QS_M4_OBJECT_NATIVE_MONGO_URI"`, `d_native_tmp="$(mktemp -d "$HOME/qs-m4-d-native-${fixture_run_owner}.XXXXXX")"`, `export TMPDIR="$d_native_tmp"`, `-ldflags="-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$object_source"`, `-run='` + committedRuntimeCompletionSelector + `' ./scripts/oneoff/m4_maintenance_table_protection | tee runtime-completion-native.jsonl`, `python3 - runtime-completion-native.jsonl "$object_source" <<'PY_D_RUNTIME_COMPLETION_PROOF'`} {
		scope := tail
		if strings.HasPrefix(required, "case ") {
			scope = run
		}
		if !strings.Contains(scope, required) {
			t.Fatal("new phase lost a finite compiled/dependency/private-root boundary", required)
		}
	}
	end := strings.Index(tail, "\nPY_D_RUNTIME_COMPLETION_PROOF\n")
	if end < 0 || strings.Count(tail[end:], "cleanup_owned_fixture\n") != 1 || !strings.HasSuffix(tail, "cleanup_owned_fixture\nunset TMPDIR\n") {
		t.Fatal("new proof must precede its own healthy release and TMPDIR reset")
	}
	selectors := regexp.MustCompile(`-run='([^']+)' ./scripts/oneoff/m4_maintenance_table_protection`).FindAllStringSubmatch(run, -1)
	if len(selectors) != 3 || selectors[2][1] != committedRuntimeCompletionSelector {
		t.Fatal("completion must be one independently anchored third D command")
	}
	selector := regexp.MustCompile(committedRuntimeCompletionSelector)
	files, e := filepath.Glob(filepath.Join(repoRoot(t), "scripts/oneoff/m4_maintenance_table_protection/native*_test.go"))
	if e != nil {
		t.Fatal(e)
	}
	matched := []string{}
	for _, file := range files {
		parsed, e := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if e != nil {
			t.Fatal(e)
		}
		for _, decl := range parsed.Decls {
			if f, ok := decl.(*ast.FuncDecl); ok && strings.HasPrefix(f.Name.Name, "Test") {
				if selector.MatchString(f.Name.Name) {
					matched = append(matched, f.Name.Name)
				}
				if selector.MatchString(f.Name.Name + "Unexpected") {
					t.Fatal("new selector overmatched")
				}
			}
		}
	}
	if len(matched) != 1 || matched[0] != "TestM4MaintenanceTablesCommittedRuntimeCompletionNative" {
		t.Fatal("new selector omitted or imported another actual test", matched)
	}
}

func TestCommittedRuntimeCompletionProofNamesMatchActualNativeCases(t *testing.T) {
	_, required := completionProof(t)
	path := filepath.Join(repoRoot(t), "scripts/oneoff/m4_maintenance_table_protection/native_runtime_completion_test.go")
	file, e := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if e != nil {
		t.Fatal(e)
	}
	const top = "TestM4MaintenanceTablesCommittedRuntimeCompletionNative"
	got := []string{top}
	for _, decl := range file.Decls {
		f, ok := decl.(*ast.FuncDecl)
		if !ok || f.Name.Name != top {
			continue
		}
		ast.Inspect(f.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Run" {
				return true
			}
			if literal, ok := call.Args[0].(*ast.BasicLit); ok {
				value, e := strconv.Unquote(literal.Value)
				if e != nil {
					t.Fatal(e)
				}
				got = append(got, top+"/"+value)
			}
			return true
		})
	}
	// These three are the single explicit mode loop in the actual new top.
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(raw), `[]string{"cancel102", "target103", "restored102"}`) || !strings.Contains(string(raw), `t.Run("CreateACKUnknown_"+mode`) {
		t.Fatal("actual completion mode loop changed without required-name review")
	}
	for _, mode := range []string{"cancel102", "target103", "restored102"} {
		got = append(got, top+"/CreateACKUnknown_"+mode)
	}
	sort.Strings(got)
	sort.Strings(required)
	if len(got) != 13 || strings.Join(got, "\n") != strings.Join(required, "\n") {
		t.Fatal("new native case omitted or foreign proof name admitted", got, required)
	}
}

func TestCommittedRuntimeCompletionProofRejectsIncompleteOrForeignEvidence(t *testing.T) {
	proof, names := completionProof(t)
	source := strings.Repeat("a", 40)
	const pkg = "github.com/FangcunMount/qs-server/scripts/oneoff/m4_maintenance_table_protection"
	eventsFor := func() []map[string]any {
		events := make([]map[string]any, 0, len(names)*2+3)
		for _, name := range names {
			events = append(events, map[string]any{"Action": "run", "Test": name, "Package": pkg}, map[string]any{"Action": "pass", "Test": name, "Package": pkg})
		}
		return append(events, map[string]any{"Action": "output", "Test": names[1], "Package": pkg, "Output": "actual compiled D fixture tool=" + source + " phase=before_dependency_binding\n"}, map[string]any{"Action": "output", "Test": names[1], "Package": pkg, "Output": "actual owned dependencies mysql=8.0.36 mongo=7.0.31 rs0; Dsource=c335b2a1a34dd69830261e2d8e11b16da3f8befb resources=20e0096ffa50a50a7818bfd09741fa48ce8d6e0dc909bf0cf49e0292ba254c4d files=286; tool=" + source + "\n"}, map[string]any{"Action": "pass", "Package": pkg})
	}
	for _, name := range []string{"complete", "missing_run", "missing_pass", "duplicate_run", "duplicate_pass", "foreign", "old_phase", "skip", "test_fail", "package_fail", "missing_package_pass", "foreign_package", "missing_entry", "wrong_entry", "missing_dependencies", "wrong_dependencies", "foreign_source", "invalid_source", "empty", "truncated_json"} {
		t.Run(name, func(t *testing.T) {
			events := eventsFor()
			switch name {
			case "missing_run":
				events = events[1:]
			case "missing_pass":
				events = append(events[:1], events[2:]...)
			case "duplicate_run":
				events = append(events, events[0])
			case "duplicate_pass":
				events = append(events, events[1])
			case "foreign":
				events = append(events, map[string]any{"Action": "run", "Test": names[0] + "Unexpected", "Package": pkg})
			case "old_phase":
				events = append(events, map[string]any{"Action": "run", "Test": "TestM4MaintenanceTablesNativeRuntimeGeneration", "Package": pkg})
			case "skip", "test_fail":
				action := "skip"
				if name == "test_fail" {
					action = "fail"
				}
				events = append(events, map[string]any{"Action": action, "Test": names[1], "Package": pkg})
			case "package_fail":
				events = append(events, map[string]any{"Action": "fail", "Package": pkg})
			case "missing_package_pass":
				events = events[:len(events)-1]
			case "foreign_package":
				events[0] = map[string]any{"Action": "run", "Test": names[0], "Package": "foreign/package"}
			case "missing_entry":
				events = append(events[:len(events)-3], events[len(events)-2:]...)
			case "wrong_entry":
				events[len(events)-3] = map[string]any{"Action": "output", "Test": names[1], "Package": pkg, "Output": "actual compiled D fixture tool=" + strings.Repeat("b", 40) + " phase=before_dependency_binding\n"}
			case "missing_dependencies":
				events = append(events[:len(events)-2], events[len(events)-1])
			case "wrong_dependencies":
				events[len(events)-2] = map[string]any{"Action": "output", "Test": names[1], "Package": pkg, "Output": "actual owned dependencies mysql=8.0.36 mongo=7.0.31 rs0; Dsource=c335b2a1a34dd69830261e2d8e11b16da3f8befb resources=20e0096ffa50a50a7818bfd09741fa48ce8d6e0dc909bf0cf49e0292ba254c4d files=285; tool=" + source + "\n"}
			case "foreign_source":
				events = append(events, map[string]any{"Action": "output", "Test": names[1], "Package": pkg, "Output": "actual compiled D fixture tool=" + strings.Repeat("b", 40) + " phase=before_dependency_binding\n"})
			case "empty":
				events = nil
			}
			path := filepath.Join(t.TempDir(), "completion.jsonl")
			file, e := os.Create(path)
			if e != nil {
				t.Fatal(e)
			}
			for _, event := range events {
				if json.NewEncoder(file).Encode(event) != nil {
					t.Fatal("proof input write failed")
				}
			}
			if name == "truncated_json" {
				if _, e := file.WriteString("{\n"); e != nil {
					t.Fatal(e)
				}
			}
			if file.Close() != nil {
				t.Fatal("proof input release failed")
			}
			boundSource := source
			if name == "invalid_source" {
				boundSource = "unknown"
			}
			output, e := exec.Command("python3", "-c", proof, path, boundSource).CombinedOutput()
			if (e == nil) != (name == "complete") {
				t.Fatalf("completion13 proof qualification mismatch %v %s", e, output)
			}
		})
	}
}

func TestCommittedRuntimeCompletionRequiresFixedActualOldCLI(t *testing.T) {
	run, _ := completionWorkflow(t)
	start := strings.Index(run, "start_owned_fixture h\n")
	if start < 0 {
		t.Fatal("completion phase missing")
	}
	tail := run[start:]
	for _, required := range []string{
		`old_completion_source='5707bfdcdbf38bb2d5cf74d45d9db04e85f49b9c'`,
		`git archive --format=tar --output="$d_native_tmp/old-source.tar" "$old_completion_source"`,
		`timeout 120s env GOMAXPROCS=2 go -C "$d_native_tmp/old-source" build -p=2`,
		`-ldflags="-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$old_completion_source"`,
		`export QS_M4_D_COMPLETION_OLD_CLI_REQUIRED=1`,
		`export QS_M4_D_COMPLETION_OLD_CLI_FILE="$d_native_tmp/old-d-cli"`,
		`export QS_M4_D_COMPLETION_OLD_CLI_SHA256`,
		`timeout 60s git fetch --no-tags --depth=1 origin "$old_completion_source"`,
		`[[ "$(git rev-parse "${old_completion_source}^{commit}")" == "$old_completion_source" ]]`,
		`env -i GOMAXPROCS=2 "$QS_M4_D_COMPLETION_OLD_CLI_FILE" --fixed-old-cli-pre-environment-probe-must-be-invalid`,
		`[[ "$old_probe_status" == 1 ]]`,
	} {
		if !strings.Contains(tail, required) {
			t.Fatal("fixed old CLI build/execution boundary absent", required)
		}
	}
	// A correct hash observed only before execution cannot certify the binary
	// actually retained by the completed phase. Preserve both source and file
	// checks around its exact native command, ahead of the existing13 proof.
	ordered := []string{
		`[[ "$(git rev-parse HEAD)" == "$object_source" ]]`,
		`git archive --format=tar`,
		`timeout 120s env GOMAXPROCS=2 go -C`,
		`QS_M4_D_COMPLETION_OLD_CLI_SHA256="$(sha256sum`,
		`env -i GOMAXPROCS=2`,
		`PY_D_OLD_CLI_PROBE` + "\n",
		`GOMAXPROCS=2 go test -p=2 -race -tags=integration -count=1 -timeout=8m -json`,
		`[[ "$(sha256sum "$QS_M4_D_COMPLETION_OLD_CLI_FILE" | awk '{print $1}')" == "$QS_M4_D_COMPLETION_OLD_CLI_SHA256" ]]`,
		`[[ "$(git rev-parse HEAD)" == "$object_source" ]]`,
		`python3 - runtime-completion-native.jsonl`,
	}
	remaining := tail
	for _, boundary := range ordered {
		_, next, ok := strings.Cut(remaining, boundary)
		if !ok {
			t.Fatal("fixed old producer proof/build/native/posthash ordering lost", boundary)
		}
		remaining = next
	}
	source, e := os.ReadFile(filepath.Join(repoRoot(t), "scripts/oneoff/m4_maintenance_table_protection/native_runtime_completion_test.go"))
	if e != nil || !strings.Contains(string(source), `const completionOldCLISource = "5707bfdcdbf38bb2d5cf74d45d9db04e85f49b9c"`) || !strings.Contains(string(source), `t.Run("ExistingOriginalMarkerAfterOutputLoss", nativeCompletionAcrossActualOldCLI)`) {
		t.Fatal("actual old native consumer and fixed CI producer disagree")
	}
}
