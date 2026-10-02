package contract_test

import (
	"path/filepath"
	"testing"
)

// This fixture verifies cross-language shape, not approved content or runtime support.
func TestMBTIThemeOutputContract(t *testing.T) {
	t.Parallel()
	schema := compileSchemaForContractTest(t, "mbti-three-topic-output", loadAIExplanationSchema(t, "ai-explanation-output-v2.schema.json"))
	value := loadJSONObject(t, filepath.Join("testdata", "mbti-three-topic-output.json"))
	if err := schema.Validate(value); err != nil {
		t.Fatalf("valid synthetic three-topic output: %v", err)
	}

	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing_topic", func(v map[string]any) { v["sections"] = v["sections"].([]any)[:2] }},
		{"duplicate_topic", func(v map[string]any) { themeSection(v, 1)["topic"] = "personality" }},
		{"wrong_scene", func(v map[string]any) { v["scene_contract_version"] = "mbti-single-assessment/v1" }},
		{"reference_as_fact", func(v map[string]any) { themeInsight(v)["basis"] = "report_fact" }},
		{"unreferenced_general", func(v map[string]any) { themeInsight(v)["reference_refs"] = []any{} }},
		{"unknown_axis", func(v map[string]any) {
			themeInsight(v)["evidence_refs"].([]any)[1].(map[string]any)["ref"] = "dimension:IQ"
		}},
		{"arbitrary_source_url", func(v map[string]any) {
			themeInsight(v)["source_url"] = "https://example.invalid/unreviewed"
		}},
		{"blank_content", func(v map[string]any) { themeInsight(v)["content"] = "  \n " }},
		{"question_as_fact", func(v map[string]any) {
			themeSection(v, 0)["reflection_questions"].([]any)[0].(map[string]any)["basis"] = "report_fact"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := cloneJSONObject(t, value)
			tc.mutate(changed)
			if err := schema.Validate(changed); err == nil {
				t.Fatal("invalid three-topic output accepted")
			}
		})
	}

	legacy := compileSchemaForContractTest(t, "legacy-output", loadAIExplanationSchema(t, "ai-explanation-output-v1.schema.json"))
	if err := legacy.Validate(value); err == nil {
		t.Fatal("legacy output schema must not silently accept the new contract")
	}
}

func themeSection(value map[string]any, index int) map[string]any {
	return value["sections"].([]any)[index].(map[string]any)
}

func themeInsight(value map[string]any) map[string]any {
	return themeSection(value, 0)["insights"].([]any)[0].(map[string]any)
}
