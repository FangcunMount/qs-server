package main

import (
	"encoding/json"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func TestBuildMBTIPayloadHasExplicitRuntime(t *testing.T) {
	payload, err := buildMBTIPayload()
	if err != nil {
		t.Fatalf("buildMBTIPayload: %v", err)
	}
	if payload.QuestionnaireVersion != "2.0.1" {
		t.Fatalf("questionnaire_version = %s, want 2.0.1", payload.QuestionnaireVersion)
	}
	runtime := payload.Runtime
	if runtime == nil || !runtime.FactorGraph.HasExplicitFactorGraph() {
		t.Fatal("expected explicit factor graph")
	}
	if runtime.Report.Kind != "personality_type" || runtime.Report.AdapterKey != "mbti" {
		t.Fatalf("report = %#v, want personality_type/mbti", runtime.Report)
	}
	if len(runtime.FactorGraph.QuestionMappings) != 32 {
		t.Fatalf("question_mappings = %d, want 32", len(runtime.FactorGraph.QuestionMappings))
	}
	for _, dim := range []string{"EI", "SN", "TF", "JP"} {
		factor, ok := runtime.FactorGraph.Factors[dim]
		if !ok {
			t.Fatalf("missing factor %s", dim)
		}
		if len(factor.Contributions) != 8 {
			t.Fatalf("%s contributions = %d, want 8", dim, len(factor.Contributions))
		}
	}
}

func TestBuildSBTIPayloadHasExplicitRuntime(t *testing.T) {
	payload, err := buildSBTIPayload()
	if err != nil {
		t.Fatalf("buildSBTIPayload: %v", err)
	}
	runtime := payload.Runtime
	if runtime == nil || !runtime.FactorGraph.HasExplicitFactorGraph() {
		t.Fatal("expected explicit factor graph")
	}
	if runtime.Decision.Kind != modelcatalog.DecisionKindNearestPattern {
		t.Fatalf("decision kind = %s", runtime.Decision.Kind)
	}
	if runtime.Report.AdapterKey != "sbti" {
		t.Fatalf("report adapter = %s, want sbti", runtime.Report.AdapterKey)
	}
}

func TestBuildMBTI93PayloadHasExplicitRuntime(t *testing.T) {
	seed, err := loadQuestionnaireSeed(mbti93QuestionnairePath)
	if err != nil {
		t.Fatalf("load mbti93 questionnaire: %v", err)
	}
	payload, err := buildMBTI93Payload()
	if err != nil {
		t.Fatalf("buildMBTI93Payload: %v", err)
	}
	if err := validatePayloadAgainstQuestionnaire(payload, seed); err != nil {
		t.Fatalf("validate mbti93 payload: %v", err)
	}
	if len(seed.Questions) != 93 {
		t.Fatalf("questions = %d, want 93", len(seed.Questions))
	}
	runtime := payload.Runtime
	if runtime == nil || !runtime.FactorGraph.HasExplicitFactorGraph() {
		t.Fatal("expected explicit factor graph")
	}
	if runtime.Decision.Kind != modelcatalog.DecisionKindPoleComposition {
		t.Fatalf("decision kind = %s", runtime.Decision.Kind)
	}
	if runtime.Report.AdapterKey != "mbti" {
		t.Fatalf("report adapter = %s, want mbti", runtime.Report.AdapterKey)
	}
	if len(payload.Outcomes) != 16 {
		t.Fatalf("outcomes = %d, want 16", len(payload.Outcomes))
	}
	wantCounts := map[string]int{"EI": 23, "SN": 23, "TF": 23, "JP": 24}
	for factor, want := range wantCounts {
		if len(runtime.FactorGraph.Factors[factor].Contributions) != want {
			t.Fatalf("%s contributions = %d, want %d", factor, len(runtime.FactorGraph.Factors[factor].Contributions), want)
		}
	}
	if runtime.FactorGraph.Dimensions["EI"].Threshold != 11.5 {
		t.Fatalf("EI threshold = %v, want 11.5", runtime.FactorGraph.Dimensions["EI"].Threshold)
	}
}

func TestBuildBig5PayloadHasExplicitRuntime(t *testing.T) {
	seed, err := loadQuestionnaireSeed(big5QuestionnairePath)
	if err != nil {
		t.Fatalf("load big5 questionnaire: %v", err)
	}
	payload, err := buildBig5Payload()
	if err != nil {
		t.Fatalf("buildBig5Payload: %v", err)
	}
	if err := validatePayloadAgainstQuestionnaire(payload, seed); err != nil {
		t.Fatalf("validate big5 payload: %v", err)
	}
	runtime := payload.Runtime
	if runtime == nil || !runtime.FactorGraph.HasExplicitFactorGraph() {
		t.Fatal("expected explicit factor graph")
	}
	if runtime.Decision.Kind != modelcatalog.DecisionKindTraitProfile {
		t.Fatalf("decision kind = %s", runtime.Decision.Kind)
	}
	if runtime.Report.AdapterKey != "bigfive" {
		t.Fatalf("report adapter = %s, want bigfive", runtime.Report.AdapterKey)
	}
	if len(runtime.FactorGraph.QuestionMappings) != 50 {
		t.Fatalf("question_mappings = %d, want 50", len(runtime.FactorGraph.QuestionMappings))
	}
	for _, code := range []string{"O", "C", "E", "A", "N"} {
		factor := runtime.FactorGraph.Factors[code]
		if len(factor.Contributions) != 10 {
			t.Fatalf("%s contributions = %d, want 10", code, len(factor.Contributions))
		}
	}
}

func TestBuildEnneagramPayloadHasExplicitRuntime(t *testing.T) {
	seed, err := loadQuestionnaireSeed(enneagramQuestionnairePath)
	if err != nil {
		t.Fatalf("load enneagram questionnaire: %v", err)
	}
	payload, err := buildEnneagramPayload()
	if err != nil {
		t.Fatalf("buildEnneagramPayload: %v", err)
	}
	if err := validatePayloadAgainstQuestionnaire(payload, seed); err != nil {
		t.Fatalf("validate enneagram payload: %v", err)
	}
	runtime := payload.Runtime
	if runtime == nil || !runtime.FactorGraph.HasExplicitFactorGraph() {
		t.Fatal("expected explicit factor graph")
	}
	if runtime.Decision.Kind != modelcatalog.DecisionKindTraitProfile {
		t.Fatalf("decision kind = %s", runtime.Decision.Kind)
	}
	if runtime.Report.AdapterKey != "trait_profile" {
		t.Fatalf("report adapter = %s, want trait_profile", runtime.Report.AdapterKey)
	}
	if len(runtime.FactorGraph.QuestionMappings) != 45 {
		t.Fatalf("question_mappings = %d, want 45", len(runtime.FactorGraph.QuestionMappings))
	}
	for _, code := range []string{"E1", "E2", "E3", "E4", "E5", "E6", "E7", "E8", "E9"} {
		factor := runtime.FactorGraph.Factors[code]
		if len(factor.Contributions) != 5 {
			t.Fatalf("%s contributions = %d, want 5", code, len(factor.Contributions))
		}
	}
}

func TestQuestionnaireSeedsAlignWithModelVersions(t *testing.T) {
	mbtiSeed, err := loadQuestionnaireSeed(mbtiQuestionnairePath)
	if err != nil {
		t.Fatalf("load mbti questionnaire: %v", err)
	}
	mbtiPayload, err := buildMBTIPayload()
	if err != nil {
		t.Fatalf("buildMBTIPayload: %v", err)
	}
	if err := validatePayloadAgainstQuestionnaire(mbtiPayload, mbtiSeed); err != nil {
		t.Fatalf("validate mbti payload: %v", err)
	}
	if mbtiSeed.Version != mbtiPayload.QuestionnaireVersion {
		t.Fatalf("mbti questionnaire version = %s, model wants %s", mbtiSeed.Version, mbtiPayload.QuestionnaireVersion)
	}

	sbtiSeed, err := loadQuestionnaireSeed(sbtiQuestionnairePath)
	if err != nil {
		t.Fatalf("load sbti questionnaire: %v", err)
	}
	sbtiPayload, err := buildSBTIPayload()
	if err != nil {
		t.Fatalf("buildSBTIPayload: %v", err)
	}
	if err := validatePayloadAgainstQuestionnaire(sbtiPayload, sbtiSeed); err != nil {
		t.Fatalf("validate sbti payload: %v", err)
	}
	if sbtiSeed.Version != sbtiPayload.QuestionnaireVersion {
		t.Fatalf("sbti questionnaire version = %s, model wants %s", sbtiSeed.Version, sbtiPayload.QuestionnaireVersion)
	}

	for _, tc := range []struct {
		name  string
		path  string
		build func() (*modeltypology.Payload, error)
	}{
		{"mbti93", mbti93QuestionnairePath, buildMBTI93Payload},
		{"big5", big5QuestionnairePath, buildBig5Payload},
		{"enneagram", enneagramQuestionnairePath, buildEnneagramPayload},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seed, err := loadQuestionnaireSeed(tc.path)
			if err != nil {
				t.Fatalf("load questionnaire: %v", err)
			}
			payload, err := tc.build()
			if err != nil {
				t.Fatalf("build payload: %v", err)
			}
			if err := validatePayloadAgainstQuestionnaire(payload, seed); err != nil {
				t.Fatalf("validate payload: %v", err)
			}
			if seed.Version != payload.QuestionnaireVersion {
				t.Fatalf("questionnaire version = %s, model wants %s", seed.Version, payload.QuestionnaireVersion)
			}
		})
	}
}

func TestPayloadDefinitionBytesStoresDraftEnvelope(t *testing.T) {
	payload, err := buildMBTIPayload()
	if err != nil {
		t.Fatalf("buildMBTIPayload: %v", err)
	}
	data, err := payloadDefinitionBytes(payload)
	if err != nil {
		t.Fatalf("payloadDefinitionBytes: %v", err)
	}
	var envelope struct {
		Algorithm string                     `json:"algorithm"`
		Outcomes  []modeltypology.Outcome    `json:"outcomes"`
		Runtime   *modeltypology.RuntimeSpec `json:"runtime"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("unmarshal draft envelope: %v", err)
	}
	if envelope.Runtime == nil || !envelope.Runtime.FactorGraph.HasExplicitFactorGraph() {
		t.Fatal("expected explicit factor graph in draft envelope")
	}
	if len(envelope.Outcomes) == 0 {
		t.Fatal("expected outcomes in draft envelope")
	}
}
