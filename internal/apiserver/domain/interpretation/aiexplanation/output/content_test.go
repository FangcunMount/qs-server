package output

import "testing"

func TestContentRejectsDuplicateEvidence(t *testing.T) {
	content := validContent()
	content.IntegratedInsights[0].EvidenceRefs[1] = content.IntegratedInsights[0].EvidenceRefs[0]
	if err := content.Validate(); err == nil {
		t.Fatal("expected duplicate evidence rejection")
	}
}

func TestStandardDerivedSuggestionRequiresSourceReference(t *testing.T) {
	content := validContent()
	content.Suggestions[0].SourceSuggestionRefs = nil
	if err := content.Validate(); err == nil {
		t.Fatal("expected missing source suggestion rejection")
	}
}

func TestContentCloneProtectsNestedSlices(t *testing.T) {
	content := validContent()
	clone := content.Clone()
	clone.IntegratedInsights[0].EvidenceRefs[0].Ref = "dimension:changed"
	clone.Suggestions[0].Actions[0] = "changed"
	if content.IntegratedInsights[0].EvidenceRefs[0].Ref == "dimension:changed" || content.Suggestions[0].Actions[0] == "changed" {
		t.Fatal("content was mutated through cloned slices")
	}
}

func validContent() Content {
	return Content{
		SchemaVersion: "ai-explanation-output/v1",
		Summary:       "本次结果呈现出两个维度之间值得关注的组合关系。",
		IntegratedInsights: []IntegratedInsight{{
			Kind: InsightKindReinforcingPattern, Title: "组合表现", Content: "两个维度在部分情境中可能相互增强。",
			WhyItMatters: "有助于理解本次结果的整体表现。",
			EvidenceRefs: []EvidenceRef{{Kind: EvidenceKindDimension, Ref: "dimension:a"}, {Kind: EvidenceKindDimension, Ref: "dimension:b"}},
		}},
		Suggestions: []Suggestion{{
			Origin: SuggestionOriginStandardDerived, Category: "daily_observation", Title: "观察情境",
			Goal: "了解组合表现出现的情境。", Actions: []string{"记录一次相关情境。"}, Rationale: "与本次结果相关。",
			EvidenceRefs:         []EvidenceRef{{Kind: EvidenceKindDimension, Ref: "dimension:a"}},
			SourceSuggestionRefs: []string{"suggestion:a"}, Caution: "可按实际情况选择。",
		}},
		Limitations: []string{"仅基于本次测评，不构成诊断或确定性判断。"},
	}
}
