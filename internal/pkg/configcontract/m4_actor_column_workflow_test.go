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

const actorColumnNativeSelector = "^(TestM4ActorColumnsNativeCaptureUpgradeVerify|TestM4ActorColumnsNativeUnboundMaterialRejected|TestM4ActorColumnsNativeRetainedAndBoundaryDrift|TestM4ActorColumnsNativeOriginalSchemaDrift|TestM4ActorColumnsNativeIncompletePrivilege|TestM4ActorColumnsNativeRestoreOriginalStates|TestM4ActorColumnsNativeRestoreContinuation|TestM4ActorColumnsNativeRestoreRefusesForeignPartialState|TestM4ActorColumnsRestoredVerificationNative)$"

var actorColumnNativeNames = []string{
	"TestM4ActorColumnsNativeCaptureUpgradeVerify",
	"TestM4ActorColumnsNativeUnboundMaterialRejected",
	"TestM4ActorColumnsNativeUnboundMaterialRejected/hash",
	"TestM4ActorColumnsNativeUnboundMaterialRejected/source",
	"TestM4ActorColumnsNativeUnboundMaterialRejected/scope",
	"TestM4ActorColumnsNativeUnboundMaterialRejected/identity",
	"TestM4ActorColumnsNativeUnboundMaterialRejected/eof",
	"TestM4ActorColumnsNativeUnboundMaterialRejected/boundary",
	"TestM4ActorColumnsNativeRetainedAndBoundaryDrift",
	"TestM4ActorColumnsNativeOriginalSchemaDrift",
	"TestM4ActorColumnsNativeIncompletePrivilege",
	"TestM4ActorColumnsNativeRestoreOriginalStates",
	"TestM4ActorColumnsNativeRestoreOriginalStates/dirty_before_drop",
	"TestM4ActorColumnsNativeRestoreOriginalStates/dirty_after_tags_drop",
	"TestM4ActorColumnsNativeRestoreOriginalStates/dirty_after_both_drops",
	"TestM4ActorColumnsNativeRestoreOriginalStates/clean_after_both_drops",
	"TestM4ActorColumnsNativeRestoreContinuation",
	"TestM4ActorColumnsNativeRestoreContinuation/roles_add_ack",
	"TestM4ActorColumnsNativeRestoreContinuation/first_roles_batch_commit_ack",
	"TestM4ActorColumnsNativeRestoreContinuation/tags_add_ack",
	"TestM4ActorColumnsNativeRestoreRefusesForeignPartialState",
	"TestM4ActorColumnsRestoredVerificationNative",
	"TestM4ActorColumnsRestoredVerificationNative/ExactOriginalReadOnly",
	"TestM4ActorColumnsRestoredVerificationNative/ReadOnlySQLAccount",
	"TestM4ActorColumnsRestoredVerificationNative/RetainedFactDriftRejected",
	"TestM4ActorColumnsRestoredVerificationNative/CommittedLedgerACKUnknown",
	"TestM4ActorColumnsRestoredVerificationNative/DifferentCapturesShareSQLGate",
	"TestM4ActorColumnsRestoredVerificationNative/ActiveRestoreMDLBlocksVerifier",
	"TestM4ActorColumnsRestoredVerificationNative/GateOwnershipLossFailsClosed",
	"TestM4ActorColumnsRestoredVerificationNative/ReleaseACKUnknownNeverComplete",
}

func TestActorColumnNativeWorkflowRequiresFreshExact30(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/workflows/ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	type runDefaults struct {
		Run struct {
			WorkingDirectory string `yaml:"working-directory"`
		} `yaml:"run"`
	}
	var workflow struct {
		Defaults runDefaults `yaml:"defaults"`
		Jobs     map[string]struct {
			Timeout  int         `yaml:"timeout-minutes"`
			Defaults runDefaults `yaml:"defaults"`
			Services map[string]struct {
				Image   string   `yaml:"image"`
				Ports   []string `yaml:"ports"`
				Options string   `yaml:"options"`
			} `yaml:"services"`
			Steps []struct {
				Run              string `yaml:"run"`
				WorkingDirectory string `yaml:"working-directory"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if yaml.Unmarshal(raw, &workflow) != nil {
		t.Fatal("native workflow is invalid")
	}
	job := workflow.Jobs["metadata-readiness-mongodb-native"]
	runs := ""
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "start_owned_fixture c\n") && step.WorkingDirectory != "" {
			t.Fatal("C native must execute from the checkout root")
		}
		runs += step.Run
	}
	if workflow.Defaults.Run.WorkingDirectory != "" || job.Defaults.Run.WorkingDirectory != "" || regexp.MustCompile(`(?m)^\s*(cd|pushd|popd)\s`).MatchString(runs) {
		t.Fatal("C native relative package and source paths require the checkout root")
	}
	if job.Timeout != 100 || strings.Count(runs, "-timeout=8m -json") != 6 {
		t.Fatal("seven fresh phases retain the100-minute job and their original eight-minute test budgets")
	}
	phases := regexp.MustCompile(`(?m)^start_owned_fixture ([mabcdgh])$`).FindAllStringSubmatch(runs, -1)
	if len(phases) != 7 {
		t.Fatal("metadata/A/B/C/D/combined/completion require seven fresh phases")
	}
	for i, phase := range []string{"m", "a", "b", "c", "d", "g", "h"} {
		if phases[i][1] != phase {
			t.Fatal("C wiring changed the original fresh-phase order")
		}
	}
	bEnd := strings.LastIndex(runs, "PY_MONGO_OBJECT_PROOF")
	cStart := strings.Index(runs, "start_owned_fixture c\n")
	cEnd := strings.LastIndex(runs, "PY_ACTOR_OBJECT_PROOF")
	dStart := strings.Index(runs, "start_owned_fixture d\n")
	if bEnd < 0 || cStart <= bEnd || cEnd <= cStart || dStart <= cEnd {
		t.Fatal("C proof must remain between completed B and fresh D")
	}
	for _, boundary := range [][2]int{{bEnd, cStart}, {cEnd, dStart}} {
		if strings.Count(runs[boundary[0]:boundary[1]], "cleanup_owned_fixture\n") != 1 {
			t.Fatal("each adjacent phase needs exactly one successful owned-fixture release")
		}
	}
	cPhase := runs[cStart:dStart]
	for _, required := range []string{
		`export QS_M4_C_OBJECT_NATIVE_REQUIRED=1`,
		`export QS_M4_C_OBJECT_NATIVE_DISPOSABLE_ACK=owned_synthetic_dependencies_only`,
		`export QS_M4_C_OBJECT_NATIVE_MYSQL_DSN="root:m4-owned-object-fixture@tcp(127.0.0.1:${OBJECT_MYSQL_PORT})/"`,
		`export QS_M4_C_OBJECT_NATIVE_MONGO_URI="$QS_M4_OBJECT_NATIVE_MONGO_URI"`,
		`GOMAXPROCS=2 go test -p=2 -race -tags=integration -count=1 -timeout=8m -json`,
		`-ldflags="-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$object_source"`,
		`python3 - actor-object-native.jsonl "$object_source" <<'PY_ACTOR_OBJECT_PROOF'`,
	} {
		if strings.Count(cPhase, required) != 1 {
			t.Fatal("C must retain its explicit administrative DSN, inherited authenticated URI, checkout source and bounded command", required)
		}
	}
	for _, required := range []string{
		`[[ "$OBJECT_MYSQL_PORT" =~ ^[0-9]+$ && "$OBJECT_MYSQL_PORT" != 3306 ]]`,
		`object_source="$(git rev-parse HEAD)"`,
		`case "$1" in m|a|b|c|d|g|h)`,
		`--publish 127.0.0.1::27017 --memory=2g --memory-swap=2g --cpus=1`,
		`--tmpfs /data/db:rw,nosuid,nodev,size=1g`,
		`--tmpfs /data/configdb:rw,nosuid,nodev,size=128m`,
		`mongo@sha256:43fddee7e532a920f3dfdee9e8f4834398c155c26bcb92d790cc1cd3c630fc40`,
		`export QS_M4_OBJECT_NATIVE_MONGO_URI="mongodb://metadata_fixture:m4-owned-mongo-fixture@127.0.0.1:${QS_MONGO_METADATA_NATIVE_PORT}/?replicaSet=rs0&directConnection=true&authSource=admin"`,
	} {
		if !strings.Contains(runs[:cStart], required) {
			t.Fatal("C must inherit the existing dynamic authenticated owned-resource boundary", required)
		}
	}
	mysql := job.Services["mysql"]
	if mysql.Image != "mysql:8.0.36" || len(mysql.Ports) != 1 || mysql.Ports[0] != "3306/tcp" || !strings.Contains(mysql.Options, "--memory=1g --cpus=1") {
		t.Fatal("C selector wiring must not enlarge or replace the existing MySQL resource")
	}
	selectors := regexp.MustCompile(`-run='([^']+)' ./scripts/oneoff/m4_actor_column_protection`).FindAllStringSubmatch(cPhase, -1)
	if len(selectors) != 1 || selectors[0][1] != actorColumnNativeSelector {
		t.Fatal("C must run exactly the nine explicitly anchored native top-level tests")
	}
	selector := regexp.MustCompile(actorColumnNativeSelector)
	tops := map[string]bool{}
	files, err := filepath.Glob(filepath.Join(repoRoot(t), "scripts/oneoff/m4_actor_column_protection/native*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && (strings.HasPrefix(function.Name.Name, "TestM4ActorColumnsNative") || function.Name.Name == "TestM4ActorColumnsRestoredVerificationNative") {
				tops[function.Name.Name] = true
			}
		}
	}
	if len(tops) != 9 {
		t.Fatal("C must retain the original eight and the restored-verification top")
	}
	for top := range tops {
		if !selector.MatchString(top) || selector.MatchString(top+"Unexpected") {
			t.Fatal("C selector omits or overmatches an actual native top", top)
		}
	}
	for _, unknown := range []string{"TestM4ActorColumnsNativeUnknown", "TestM4ActorColumnsRestoredVerificationNativeUnknown", "TestM4ActorColumnsPinnedResources"} {
		if selector.MatchString(unknown) {
			t.Fatal("C selector accepts an unreviewed or source-only test", unknown)
		}
	}
	marker := "<<'PY_ACTOR_OBJECT_PROOF'\n"
	_, proof, ok := strings.Cut(cPhase, marker)
	if !ok {
		t.Fatal("C proof executable body missing")
	}
	proof, _, ok = strings.Cut(proof, "\nPY_ACTOR_OBJECT_PROOF")
	if !ok {
		t.Fatal("C proof terminator missing")
	}
	expected := map[string]int{}
	for _, name := range actorColumnNativeNames {
		if !tops[strings.Split(name, "/")[0]] {
			t.Fatal("fixed C list refers to a nonexistent native parent", name)
		}
		expected[name]++
	}
	actual := map[string]int{}
	for _, match := range regexp.MustCompile(`(?m)^\s+'(TestM4ActorColumns[^']*)',?$`).FindAllStringSubmatch(proof, -1) {
		actual[match[1]]++
	}
	if len(expected) != 30 || len(actual) != 30 {
		t.Fatal("C proof must preserve all original21 and restored-verification9 names")
	}
	for name, count := range expected {
		if count != 1 || actual[name] != 1 {
			t.Fatal("C proof changed or duplicated an exact native selector", name)
		}
	}
	checkActorColumnNativeProof(t, proof)
}

func checkActorColumnNativeProof(t *testing.T, proof string) {
	t.Helper()
	source := strings.Repeat("a", 40)
	dependency := "actual owned dependencies mysql=8.0.36 mongo=7.0.31 rs0; Csource=040b122d7dfe77b1119d9ed6969a54abd6a9fbdf resources=fadbb5a33baed5b2fb40a6f1efdf4a20dc5a1525ab2a0090486b7d5832437814 files=284; tool=" + source + "\n"
	eventsFor := func(names []string) []map[string]any {
		events := make([]map[string]any, 0, len(names)*2+1)
		for _, name := range names {
			events = append(events, map[string]any{"Action": "run", "Test": name}, map[string]any{"Action": "pass", "Test": name})
		}
		return append(events, map[string]any{"Action": "output", "Test": names[0], "Output": dependency})
	}
	base := eventsFor(actorColumnNativeNames)
	for _, name := range []string{
		"complete", "missing_run", "missing_pass", "duplicate_run", "duplicate_pass", "unknown_top", "unknown_child",
		"skip_required", "fail_required", "skip_foreign", "fail_foreign", "skip_package", "fail_package",
		"wrong_compiled_source", "mixed_compiled_source", "missing_dependency_entry", "wrong_fixed_csource", "wrong_resources", "wrong_resource_count", "wrong_mysql", "wrong_mongo", "missing_rs0", "truncated_entry", "wrong_argument_source",
		"old21_only_same_source", "new9_only_same_source", "old21_new9_mixed_source", "missing_new_top_run", "missing_new_child_pass", "duplicate_new_top",
	} {
		t.Run(name, func(t *testing.T) {
			events := append([]map[string]any{}, base...)
			argumentSource := source
			filter := func(test, action string) {
				filtered := make([]map[string]any, 0, len(events))
				for _, event := range events {
					if event["Test"] != test || event["Action"] != action {
						filtered = append(filtered, event)
					}
				}
				events = filtered
			}
			changeDependency := func(old, replacement string) {
				events[len(events)-1] = map[string]any{"Action": "output", "Test": actorColumnNativeNames[0], "Output": strings.Replace(dependency, old, replacement, 1)}
			}
			switch name {
			case "missing_run", "missing_pass":
				filter(actorColumnNativeNames[0], strings.TrimPrefix(name, "missing_"))
			case "duplicate_run", "duplicate_pass":
				events = append(events, map[string]any{"Action": strings.TrimPrefix(name, "duplicate_"), "Test": actorColumnNativeNames[0]})
			case "unknown_top":
				events = append(events, map[string]any{"Action": "run", "Test": "TestM4ActorColumnsNativeUnknown"})
			case "unknown_child":
				events = append(events, map[string]any{"Action": "pass", "Test": actorColumnNativeNames[21] + "/Unreviewed"})
			case "skip_required", "fail_required", "skip_foreign", "fail_foreign", "skip_package", "fail_package":
				action, target, _ := strings.Cut(name, "_")
				event := map[string]any{"Action": action}
				switch target {
				case "required":
					event["Test"] = actorColumnNativeNames[0]
				case "foreign":
					event["Test"] = "TestOutsideTheRequiredSelector"
				}
				events = append(events, event)
			case "wrong_compiled_source":
				changeDependency(source, strings.Repeat("b", 40))
			case "mixed_compiled_source", "old21_new9_mixed_source":
				events = append(events, map[string]any{"Action": "output", "Test": actorColumnNativeNames[21], "Output": strings.Replace(dependency, source, strings.Repeat("b", 40), 1)})
			case "missing_dependency_entry":
				events = events[:len(events)-1]
			case "wrong_fixed_csource":
				changeDependency("040b122d7dfe77b1119d9ed6969a54abd6a9fbdf", strings.Repeat("c", 40))
			case "wrong_resources":
				changeDependency("fadbb5a33baed5b2fb40a6f1efdf4a20dc5a1525ab2a0090486b7d5832437814", strings.Repeat("d", 64))
			case "wrong_resource_count":
				changeDependency("files=284", "files=286")
			case "wrong_mysql":
				changeDependency("mysql=8.0.36", "mysql=8.0.37")
			case "wrong_mongo":
				changeDependency("mongo=7.0.31", "mongo=7.0.25")
			case "missing_rs0":
				changeDependency(" rs0;", ";")
			case "truncated_entry":
				events[len(events)-1] = map[string]any{"Action": "output", "Test": actorColumnNativeNames[0], "Output": "; tool=" + source + "\n"}
			case "wrong_argument_source":
				argumentSource = strings.Repeat("b", 40)
			case "old21_only_same_source":
				events = eventsFor(actorColumnNativeNames[:21])
			case "new9_only_same_source":
				events = eventsFor(actorColumnNativeNames[21:])
			case "missing_new_top_run":
				filter(actorColumnNativeNames[21], "run")
			case "missing_new_child_pass":
				filter(actorColumnNativeNames[29], "pass")
			case "duplicate_new_top":
				events = append(events, map[string]any{"Action": "run", "Test": actorColumnNativeNames[21]})
			}
			path := filepath.Join(t.TempDir(), "proof.jsonl")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range events {
				if err := json.NewEncoder(file).Encode(event); err != nil {
					_ = file.Close()
					t.Fatal(err)
				}
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			output, err := exec.Command("python3", "-c", proof, path, argumentSource).CombinedOutput()
			if (name == "complete") != (err == nil) {
				t.Fatalf("C proof must require fresh exact30, the fixed dependency/source sentinel and no global FAIL/SKIP: %v %s", err, output)
			}
		})
	}
}
