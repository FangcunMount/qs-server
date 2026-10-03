package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestAIExplanationOutputCannotAddScoresOrClassifications(t *testing.T) {
	t.Parallel()

	schema := loadAIExplanationSchema(t, "ai-explanation-output-v1.schema.json")
	properties := objectAt(t, schema, "properties")
	for _, forbidden := range []string{"score", "level", "risk", "diagnosis", "treatment"} {
		if _, ok := properties[forbidden]; ok {
			t.Fatalf("output root must not define %q", forbidden)
		}
	}

	insights := objectAt(t, properties, "integrated_insights")
	assertNumberValue(t, insights, "minItems", 1)
	evidence := objectAt(t, schema, "$defs", "integratedInsight", "properties", "evidence_refs")
	assertNumberValue(t, evidence, "minItems", 2)

	suggestion := objectAt(t, schema, "$defs", "suggestion")
	assertRequiredFields(t, suggestion, []string{
		"origin", "category", "title", "goal", "actions", "rationale",
		"evidence_refs", "source_suggestion_refs", "caution",
	})
}

func loadAIExplanationSchema(t *testing.T, name string) map[string]any {
	t.Helper()
	path := filepath.Join(repoRoot(t), "api", "schema", "interpretation", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return schema
}

func loadJSONObject(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return object
}

func compileSchemaForContractTest(t *testing.T, name string, document map[string]any) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource(name, document); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	compiled, err := compiler.Compile(name)
	if err != nil {
		t.Fatalf("compile %s: %v", name, err)
	}
	return compiled
}

func cloneJSONObject(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON object clone: %v", err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		t.Fatalf("unmarshal JSON object clone: %v", err)
	}
	return cloned
}

func objectAt(t *testing.T, root map[string]any, path ...string) map[string]any {
	t.Helper()
	current := root
	for _, segment := range path {
		value, ok := current[segment]
		if !ok {
			t.Fatalf("missing object path %s", strings.Join(path, "."))
		}
		next, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("path %s is %T, want object", strings.Join(path, "."), value)
		}
		current = next
	}
	return current
}

func assertNumberValue(t *testing.T, object map[string]any, key string, want float64) {
	t.Helper()
	got, ok := object[key].(float64)
	if !ok || got != want {
		t.Fatalf("%s = %#v, want %v", key, object[key], want)
	}
}

func assertRequiredFields(t *testing.T, object map[string]any, want []string) {
	t.Helper()
	raw, ok := object["required"].([]any)
	if !ok {
		t.Fatalf("required = %T, want array", object["required"])
	}
	got := make([]string, 0, len(raw))
	for _, item := range raw {
		value, ok := item.(string)
		if !ok {
			t.Fatalf("required item = %T, want string", item)
		}
		got = append(got, value)
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("required = %v, want %v", got, want)
	}
}
