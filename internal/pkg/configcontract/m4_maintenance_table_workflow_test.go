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

func TestMaintenanceTableNativeWorkflowRequiresFreshExact53(t *testing.T) {
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
	if job.Timeout != 60 {
		t.Fatal("four bounded native phases require the approved finite job budget")
	}
	for _, required := range []string{
		`case "$1" in m|a|b|d)`,
		`start_owned_fixture d`,
		`export QS_M4_D_OBJECT_NATIVE_REQUIRED=1`,
		`export QS_M4_D_OBJECT_NATIVE_DISPOSABLE_ACK=owned_synthetic_dependencies_only`,
		`export QS_M4_D_OBJECT_NATIVE_MYSQL_DSN="root:m4-owned-object-fixture@tcp(127.0.0.1:${OBJECT_MYSQL_PORT})/?multiStatements=true"`,
		`export QS_M4_D_OBJECT_NATIVE_MONGO_URI="$QS_M4_OBJECT_NATIVE_MONGO_URI"`,
		`d_native_tmp="$(mktemp -d "$HOME/qs-m4-d-native-${fixture_run_owner}.XXXXXX")"`,
		`export TMPDIR="$d_native_tmp"`,
		`-run='^TestM4MaintenanceTablesNative' ./scripts/oneoff/m4_maintenance_table_protection`,
		`-ldflags="-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$object_source"`,
		`-run='^TestM4SQLObjectNative'`, `-run='^TestM4MongoObjectNative'`,
		`make test-metadata-readiness-mongodb-native`,
	} {
		if !strings.Contains(runs, required) {
			t.Fatal("D native phase lost its real selector or owned dependency boundary", required)
		}
	}
	bEnd := strings.LastIndex(runs, "PY_MONGO_OBJECT_PROOF")
	dStart := strings.Index(runs, "          start_owned_fixture d")
	// YAML strips common indentation from this shell block.
	if dStart < 0 {
		dStart = strings.Index(runs, "start_owned_fixture d")
	}
	if bEnd < 0 || dStart <= bEnd || !strings.Contains(runs[bEnd:dStart], "cleanup_owned_fixture") {
		t.Fatal("D must get a fresh Mongo phase after B has released its own process")
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
	names := regexp.MustCompile(`(?m)^\s+'(TestM4MaintenanceTablesNative[^']*)',?$`).FindAllStringSubmatch(proof, -1)
	if len(names) != 53 {
		t.Fatal("D proof must enumerate all exact 53 names")
	}
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
		if expected[name[1]] || !tops[strings.Split(name[1], "/")[0]] {
			t.Fatal("D proof has duplicate or nonexistent parent selectors")
		}
		expected[name[1]] = true
	}
	if !expected["TestM4MaintenanceTablesNativeFirstRuntimeGate"] {
		t.Fatal("D proof must require the actual first-runtime gate top-level test")
	}
	if len(tops) != 16 {
		t.Fatal("D proof must retain the actual sixteen top-level tests")
	}

	source := strings.Repeat("a", 40)
	events := make([]map[string]any, 0, 94)
	for _, name := range names {
		events = append(events, map[string]any{"Action": "run", "Test": name[1]}, map[string]any{"Action": "pass", "Test": name[1]})
	}
	events = append(events,
		map[string]any{"Action": "output", "Test": names[0][1], "Output": "actual compiled D fixture tool=" + source + " phase=before_dependency_binding\n"},
		map[string]any{"Action": "output", "Test": names[0][1], "Output": "actual owned dependencies mysql=8.0.36 mongo=7.0.31 rs0; Dsource=c335b2a1a34dd69830261e2d8e11b16da3f8befb resources=20e0096ffa50a50a7818bfd09741fa48ce8d6e0dc909bf0cf49e0292ba254c4d files=286; tool=" + source + "\n"},
	)
	for _, name := range []string{"complete", "missing_run", "missing_pass", "duplicate", "unknown", "skip", "fail", "wrong_source", "missing_entry"} {
		t.Run(name, func(t *testing.T) {
			changed := append([]map[string]any{}, events...)
			switch name {
			case "missing_run":
				changed = changed[1:]
			case "missing_pass":
				changed = append(changed[:1], changed[2:]...)
			case "duplicate":
				changed = append(changed, events[0])
			case "unknown":
				changed = append(changed, map[string]any{"Action": "run", "Test": "TestM4MaintenanceTablesNativeUnknown"})
			case "skip", "fail":
				changed = append(changed, map[string]any{"Action": name, "Test": names[0][1]})
			case "wrong_source":
				changed[len(changed)-1] = map[string]any{"Action": "output", "Test": names[0][1], "Output": strings.ReplaceAll(events[len(events)-1]["Output"].(string), source, strings.Repeat("b", 40))}
			case "missing_entry":
				changed = append(changed[:len(changed)-2], changed[len(changed)-1])
			}
			path := filepath.Join(t.TempDir(), "proof.jsonl")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range changed {
				if json.NewEncoder(file).Encode(event) != nil {
					t.Fatal("could not save local proof input")
				}
			}
			if file.Close() != nil {
				t.Fatal("local proof input release failed")
			}
			output, err := exec.Command("python3", "-c", proof, path, source).CombinedOutput()
			if (name == "complete") != (err == nil) {
				t.Fatalf("native proof did not enforce its exact required evidence: %v %s", err, output)
			}
		})
	}
}
