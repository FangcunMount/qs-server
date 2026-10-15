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

const actorRuntimeGenerationSelector = "^TestM4ActorColumnsNativeRuntimeGeneration$"

func actorRuntimeWorkflow(t *testing.T) (string, int) {
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

func actorRuntimeProof(t *testing.T) (string, []string) {
	t.Helper()
	run, _ := actorRuntimeWorkflow(t)
	_, proof, ok := strings.Cut(run, "<<'PY_C_RUNTIME_GENERATION_PROOF'\n")
	if !ok {
		t.Fatal("C generation14 has no independent executable proof")
	}
	proof, _, ok = strings.Cut(proof, "\nPY_C_RUNTIME_GENERATION_PROOF")
	if !ok {
		t.Fatal("C generation14 proof not closed")
	}
	names := regexp.MustCompile(`(?m)^    '(TestM4ActorColumnsNativeRuntimeGeneration[^']*)',$`).FindAllStringSubmatch(proof, -1)
	if len(names) != 14 {
		t.Fatal("completion proof must name top plus13 children")
	}
	required := make([]string, 0, 14)
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

func TestActorRuntimeGenerationNativeWorkflowHasSeparateFreshPhase(t *testing.T) {
	run, timeout := actorRuntimeWorkflow(t)
	if timeout != ((8*8+8*3+20+9)/10)*10 {
		t.Fatal("eight bounded phases need64m tests+24m readiness+20m setup/proof/cleanup; ceil110")
	}
	phases := regexp.MustCompile(`(?m)^start_owned_fixture ([mabcdghi])$`).FindAllStringSubmatch(run, -1)
	if len(phases) != 8 {
		t.Fatal("metadata,A47,B37,C30,D53,G21,H13,I14 need eight fresh phases")
	}
	for i, name := range []string{"m", "a", "b", "c", "d", "g", "h", "i"} {
		if phases[i][1] != name {
			t.Fatal("original phases lost or reordered")
		}
	}
	if strings.Count(run, "-timeout=8m -json") != 7 {
		t.Fatal("seven object commands plus metadata retain eight finite8m budgets")
	}
	prior := strings.LastIndex(run, "\nPY_D_RUNTIME_COMPLETION_PROOF\n")
	start := strings.Index(run, "start_owned_fixture i\n")
	if prior < 0 || start <= prior || strings.Count(run[prior:start], "cleanup_owned_fixture\n") != 1 || strings.Count(run[prior:start], "unset TMPDIR\n") != 1 {
		t.Fatal("fresh C14 must follow successful H proof and exact release/reset")
	}
	tail := run[start:]
	for _, required := range []string{
		`case "$1" in m|a|b|c|d|g|h|i)`,
		`export QS_M4_C_OBJECT_NATIVE_REQUIRED=1`,
		`export QS_M4_C_OBJECT_NATIVE_DISPOSABLE_ACK=owned_synthetic_dependencies_only`,
		`export QS_M4_C_OBJECT_NATIVE_MYSQL_DSN="root:m4-owned-object-fixture@tcp(127.0.0.1:${OBJECT_MYSQL_PORT})/"`,
		`export QS_M4_C_OBJECT_NATIVE_MONGO_URI="$QS_M4_OBJECT_NATIVE_MONGO_URI"`,
		`-ldflags="-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$object_source"`,
		`-run='` + actorRuntimeGenerationSelector + `' ./scripts/oneoff/m4_actor_column_protection | tee actor-runtime-generation-native.jsonl`,
		`python3 - actor-runtime-generation-native.jsonl "$object_source" <<'PY_C_RUNTIME_GENERATION_PROOF'`,
	} {
		scope := tail
		if strings.HasPrefix(required, "case ") {
			scope = run
		}
		if strings.Count(scope, required) != 1 {
			t.Fatal("new C phase lost exact source/adminDSN/authURI or finite command", required)
		}
	}
	end := strings.Index(tail, "\nPY_C_RUNTIME_GENERATION_PROOF\n")
	if end < 0 || strings.Count(tail[end:], "cleanup_owned_fixture\n") != 1 || !strings.HasSuffix(tail, "cleanup_owned_fixture\n") {
		t.Fatal("C14 proof must precede its own exact cleanup")
	}
	selectors := regexp.MustCompile(`-run='([^']+)' ./scripts/oneoff/m4_actor_column_protection`).FindAllStringSubmatch(run, -1)
	if len(selectors) != 2 || selectors[0][1] != actorColumnNativeSelector || selectors[1][1] != actorRuntimeGenerationSelector {
		t.Fatal("new C top must remain separate from unchanged original30 selector")
	}
	selector := regexp.MustCompile(actorRuntimeGenerationSelector)
	files, e := filepath.Glob(filepath.Join(repoRoot(t), "scripts/oneoff/m4_actor_column_protection/native*_test.go"))
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
					t.Fatal("C14 selector overmatches")
				}
			}
		}
	}
	if len(matched) != 1 || matched[0] != "TestM4ActorColumnsNativeRuntimeGeneration" {
		t.Fatal("new C selector imported or omitted actual top", matched)
	}
}

func TestActorRuntimeGenerationProofNamesMatchActualNativeCases(t *testing.T) {
	_, required := actorRuntimeProof(t)
	path := filepath.Join(repoRoot(t), "scripts/oneoff/m4_actor_column_protection/native_runtime_boundary_test.go")
	file, e := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if e != nil {
		t.Fatal(e)
	}
	const top = "TestM4ActorColumnsNativeRuntimeGeneration"
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
	// The only dynamic children are the two explicit mode/foreign-object loops.
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(raw), `[]string{"cancel101", "restored101"}`) || !strings.Contains(string(raw), `t.Run("NewT0After_"+mode`) || !strings.Contains(string(raw), `[]string{"ForeignTable", "ForeignView", "EmptyTable", "MalformedRecord"}`) || !strings.Contains(string(raw), `t.Run(kind,`) {
		t.Fatal("C14 dynamic loop changed without exact-name review")
	}
	for _, mode := range []string{"cancel101", "restored101"} {
		got = append(got, top+"/NewT0After_"+mode)
	}
	for _, kind := range []string{"ForeignTable", "ForeignView", "EmptyTable", "MalformedRecord"} {
		got = append(got, top+"/"+kind)
	}
	// The entry comes from the linked Go version symbol before any dependencies.
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == top {
			if len(f.Body.List) == 0 {
				t.Fatal("new native top missing entry")
			}
			stmt, ok := f.Body.List[0].(*ast.ExprStmt)
			if !ok {
				t.Fatal("entry must precede fixture binding")
			}
			call, ok := stmt.X.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				t.Fatal("entry is not an actual linked-source log")
			}
			logger, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || logger.Sel.Name != "Logf" {
				t.Fatal("compiled entry must be emitted by the actual test logger")
			}
			owner, ok := logger.X.(*ast.Ident)
			if !ok || owner.Name != "t" {
				t.Fatal("compiled entry logger is not the actual top")
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				t.Fatal("entry format is not fixed")
			}
			format, _ := strconv.Unquote(literal.Value)
			sel, ok := call.Args[1].(*ast.SelectorExpr)
			if !ok {
				t.Fatal("entry did not read linked version")
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "version" || sel.Sel.Name != "GitCommit" || format != "actual compiled C fixture tool=%s phase=before_dependency_binding" {
				t.Fatal("entry not tied to compiled source")
			}
		}
	}
	sort.Strings(got)
	sort.Strings(required)
	if len(got) != 14 || strings.Join(got, "\n") != strings.Join(required, "\n") {
		t.Fatal("new C native case omitted or foreign proof name admitted", got, required)
	}
}

func TestActorRuntimeGenerationProofRejectsIncompleteOrForeignEvidence(t *testing.T) {
	proof, names := actorRuntimeProof(t)
	source := strings.Repeat("a", 40)
	const pkg = "github.com/FangcunMount/qs-server/scripts/oneoff/m4_actor_column_protection"
	eventsFor := func() []map[string]any {
		events := make([]map[string]any, 0, len(names)*2+3)
		for _, name := range names {
			events = append(events, map[string]any{"Action": "run", "Test": name, "Package": pkg}, map[string]any{"Action": "pass", "Test": name, "Package": pkg})
		}
		return append(events, map[string]any{"Action": "output", "Test": names[0], "Package": pkg, "Output": "actual compiled C fixture tool=" + source + " phase=before_dependency_binding\n"}, map[string]any{"Action": "output", "Test": names[1], "Package": pkg, "Output": "actual owned dependencies mysql=8.0.36 mongo=7.0.31 rs0; Csource=040b122d7dfe77b1119d9ed6969a54abd6a9fbdf resources=fadbb5a33baed5b2fb40a6f1efdf4a20dc5a1525ab2a0090486b7d5832437814 files=284; tool=" + source + "\n"}, map[string]any{"Action": "pass", "Package": pkg})
	}
	for _, name := range []string{"complete", "missing_run", "missing_pass", "duplicate_run", "duplicate_pass", "foreign", "old_phase", "skip", "test_fail", "package_fail", "missing_package_pass", "foreign_package", "missing_entry", "wrong_entry", "missing_dependencies", "wrong_dependencies", "foreign_source", "invalid_source", "empty", "truncated_json", "foreign_output", "missing_package_property", "wrong_source_argument", "mixed_dependencies", "wrong_csource", "wrong_resources", "wrong_mysql", "wrong_mongo", "wrong_rs0", "fragmented_positive"} {
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
				events = append(events, map[string]any{"Action": "run", "Test": "TestM4ActorColumnsNativeCaptureUpgradeVerify", "Package": pkg})
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
				events[len(events)-3] = map[string]any{"Action": "output", "Test": names[0], "Package": pkg, "Output": "actual compiled C fixture tool=" + strings.Repeat("b", 40) + " phase=before_dependency_binding\n"}
			case "missing_dependencies":
				events = append(events[:len(events)-2], events[len(events)-1])
			case "wrong_dependencies":
				events[len(events)-2] = map[string]any{"Action": "output", "Test": names[1], "Package": pkg, "Output": "actual owned dependencies mysql=8.0.36 mongo=7.0.31 rs0; Csource=040b122d7dfe77b1119d9ed6969a54abd6a9fbdf resources=fadbb5a33baed5b2fb40a6f1efdf4a20dc5a1525ab2a0090486b7d5832437814 files=283; tool=" + source + "\n"}
			case "foreign_source":
				events = append(events, map[string]any{"Action": "output", "Test": names[0], "Package": pkg, "Output": "actual compiled C fixture tool=" + strings.Repeat("b", 40) + " phase=before_dependency_binding\n"})
			case "foreign_output":
				events = append(events, map[string]any{"Action": "output", "Test": names[0] + "Unexpected", "Package": pkg, "Output": "foreign\n"})
			case "missing_package_property":
				delete(events[0], "Package")
			case "mixed_dependencies":
				events = append(events, map[string]any{"Action": "output", "Test": names[1], "Package": pkg, "Output": strings.Replace(events[len(events)-2]["Output"].(string), source, strings.Repeat("b", 40), 1)})
			case "wrong_csource", "wrong_resources", "wrong_mysql", "wrong_mongo", "wrong_rs0":
				old, replacement := "", ""
				switch name {
				case "wrong_csource":
					old, replacement = "040b122d7dfe77b1119d9ed6969a54abd6a9fbdf", source
				case "wrong_resources":
					old, replacement = "fadbb5a33baed5b2fb40a6f1efdf4a20dc5a1525ab2a0090486b7d5832437814", strings.Repeat("b", 64)
				case "wrong_mysql":
					old, replacement = "mysql=8.0.36", "mysql=8.4.0"
				case "wrong_mongo":
					old, replacement = "mongo=7.0.31", "mongo=7.0.30"
				case "wrong_rs0":
					old, replacement = "rs0;", "other;"
				}
				events[len(events)-2]["Output"] = strings.Replace(events[len(events)-2]["Output"].(string), old, replacement, 1)
			case "fragmented_positive":
				head := append([]map[string]any{}, events[:len(events)-3]...)
				for _, event := range events[len(events)-3 : len(events)-1] {
					text := event["Output"].(string)
					cut := len(text) / 2
					for _, part := range []string{text[:cut], text[cut:]} {
						head = append(head, map[string]any{"Action": "output", "Test": event["Test"], "Package": pkg, "Output": part})
					}
				}
				events = append(head, events[len(events)-1])
			case "empty":
				events = nil
			}
			path := filepath.Join(t.TempDir(), "generation.jsonl")
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
			} else if name == "wrong_source_argument" {
				boundSource = strings.Repeat("b", 40)
			}
			output, e := exec.Command("python3", "-c", proof, path, boundSource).CombinedOutput()
			if (e == nil) != (name == "complete" || name == "fragmented_positive") {
				t.Fatalf("C generation14 proof qualification mismatch %v %s", e, output)
			}
		})
	}
}
