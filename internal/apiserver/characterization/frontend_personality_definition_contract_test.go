package characterization_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	configured "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/configured"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestFrontendDefinitionV2ContractValidatesBuildsAndExecutes(t *testing.T) {
	fixturePath, ok := frontendContractFixture("personalityDefinitionV2.contract.json")
	if !ok {
		t.Skip("qs-operating-system checkout is not present; cross-repository contract runs when both repositories are checked out")
	}
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read frontend contract: %v", err)
	}
	var definition modeldefinition.Definition
	if err := json.Unmarshal(raw, &definition); err != nil {
		t.Fatalf("decode frontend DefinitionV2: %v", err)
	}
	if issues := modeldefinition.Validate(definition); len(issues) != 0 {
		t.Fatalf("server DefinitionV2 validation issues: %#v", issues)
	}

	payload, err := modeltypology.PayloadFromDefinition(modeltypology.DefinitionEnvelope{
		Code: "FRONTEND_CONTRACT", Version: "v1", Title: "Frontend contract",
		QuestionnaireCode: "q_contract", QuestionnaireVersion: "v1",
		Status: "published", Algorithm: modelcatalog.AlgorithmPersonalityTypology,
	}, &definition)
	if err != nil {
		t.Fatalf("project runtime payload: %v", err)
	}
	questionnaire := modeltypology.QuestionnaireSnapshot{Questions: []modeltypology.QuestionSnapshot{
		{Code: "q_drive", Type: "Radio", OptionCodes: []string{"low", "high"}},
		{Code: "q_care", Type: "Radio", OptionCodes: []string{"low", "high"}},
	}}
	if issues := modeltypology.ValidateRuntimeSpecForPublishWithContext(payload.Runtime, questionnaire, modeltypology.RuntimeSpecValidationContext{
		Algorithm: payload.Algorithm, Outcomes: payload.Outcomes,
	}); modelcatalog.HasValidationErrors(issues) {
		t.Fatalf("server publish validation issues: %#v", issues)
	}

	model, err := modelcatalog.NewAssessmentModel(modelcatalog.NewAssessmentModelInput{
		Code: "FRONTEND_CONTRACT", Kind: modelcatalog.KindTypology, SubKind: modelcatalog.SubKindTypology,
		Algorithm: modelcatalog.AlgorithmPersonalityTypology, Title: "Frontend contract", Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("new assessment model: %v", err)
	}
	model.DefinitionV2 = &definition
	frozen, err := (appdefinition.RuntimeMaterializer{}).MaterializeTypologyRuntime(model, "published")
	if err != nil {
		t.Fatalf("materialize published runtime: %v", err)
	}
	result, err := configured.NewEvaluator().Score(frozen, &definition, &evalinput.AnswerSheet{
		QuestionnaireCode: "q_contract", QuestionnaireVersion: "v1",
		Answers: []evalinput.Answer{{QuestionCode: "q_drive", Score: 0, Value: "high"}, {QuestionCode: "q_care", Score: -2.5, Value: "low"}},
	})
	if err != nil {
		t.Fatalf("execute simulated answer sheet: %v", err)
	}
	if result.SelectedOutcome.Code != "care" || len(result.Candidate.RankedFactors) != 2 {
		t.Fatalf("unexpected execution result: %#v", result)
	}
	if got := result.Vector.Scores["drive"].Raw; got != 0 {
		t.Fatalf("drive score = %v, want 0", got)
	}
	if got := result.Vector.Scores["care"].Raw; got != 1.25 {
		t.Fatalf("care score = %v, want 1.25", got)
	}
}

func TestFrontendOptionOverrideContractExecutesWithSignAndWeight(t *testing.T) {
	definition, ok := loadFrontendDefinition(t, "personalityDefinitionV2.optionOverride.contract.json")
	if !ok {
		t.Skip("qs-operating-system checkout is not present")
	}
	payload := frontendRuntimePayload(t, definition, "FRONTEND_OVERRIDE")
	questionnaire := modeltypology.QuestionnaireSnapshot{Questions: []modeltypology.QuestionSnapshot{{Code: "q_override", Type: "Radio", OptionCodes: []string{"A", "B"}}}}
	if issues := modeltypology.ValidateRuntimeSpecForPublishWithContext(payload.Runtime, questionnaire, modeltypology.RuntimeSpecValidationContext{Algorithm: payload.Algorithm, Outcomes: payload.Outcomes}); modelcatalog.HasValidationErrors(issues) {
		t.Fatalf("publish validation issues: %#v", issues)
	}
	result, err := configured.NewEvaluator().Score(payload, definition, &evalinput.AnswerSheet{Answers: []evalinput.Answer{{QuestionCode: "q_override", Value: "A", Score: 99}}})
	if err != nil {
		t.Fatalf("execute option override: %v", err)
	}
	if got := result.Vector.Scores["override"].Raw; got != -2 {
		t.Fatalf("override score = %v, want -2", got)
	}
	if _, err := configured.NewEvaluator().Score(payload, definition, &evalinput.AnswerSheet{Answers: []evalinput.Answer{{QuestionCode: "q_override", Value: "X", Score: 99}}}); err == nil {
		t.Fatal("unknown override option must fail")
	}
}

func TestFrontendImplicitScoringContractIsRejected(t *testing.T) {
	definition, ok := loadFrontendDefinition(t, "personalityDefinitionV2.legacy.contract.json")
	if !ok {
		t.Skip("qs-operating-system checkout is not present")
	}
	payload := frontendRuntimePayload(t, definition, "FRONTEND_LEGACY")
	questionnaire := modeltypology.QuestionnaireSnapshot{Questions: []modeltypology.QuestionSnapshot{{Code: "q_legacy", Type: "Radio", OptionCodes: []string{"A", "B"}}}}
	issues := modeltypology.ValidateRuntimeSpecForPublishWithContext(payload.Runtime, questionnaire, modeltypology.RuntimeSpecValidationContext{Algorithm: payload.Algorithm, Outcomes: payload.Outcomes})
	if !modelcatalog.HasValidationErrors(issues) || !hasIssueCode(issues, "scoring_mode.required") {
		t.Fatalf("issues = %#v, want blocking scoring_mode.required", issues)
	}
	if _, err := configured.NewEvaluator().Score(payload, definition, &evalinput.AnswerSheet{Answers: []evalinput.Answer{{QuestionCode: "q_legacy", Value: "A", Score: 2}}}); err == nil {
		t.Fatal("implicit scoring definition executed")
	}
}

func loadFrontendDefinition(t *testing.T, name string) (*modeldefinition.Definition, bool) {
	t.Helper()
	path, ok := frontendContractFixture(name)
	if !ok {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read frontend contract: %v", err)
	}
	var definition modeldefinition.Definition
	if err := json.Unmarshal(raw, &definition); err != nil {
		t.Fatalf("decode frontend DefinitionV2: %v", err)
	}
	if issues := modeldefinition.Validate(definition); len(issues) != 0 {
		t.Fatalf("DefinitionV2 validation issues: %#v", issues)
	}
	return &definition, true
}

func frontendRuntimePayload(t *testing.T, definition *modeldefinition.Definition, code string) *modeltypology.Payload {
	t.Helper()
	payload, err := modeltypology.PayloadFromDefinition(modeltypology.DefinitionEnvelope{
		Code: code, Version: "v1", Title: code, QuestionnaireCode: "q_contract", QuestionnaireVersion: "v1",
		Status: "published", Algorithm: modelcatalog.AlgorithmPersonalityTypology,
	}, definition)
	if err != nil {
		t.Fatalf("project runtime payload: %v", err)
	}
	return payload
}

func hasIssueCode(issues []modelcatalog.DomainValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func frontendContractFixture(name string) (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if filepath.Base(dir) == "workspace" {
			candidate := filepath.Join(dir, "typescript", "github.com", "fangcun-mount", "qs-operating-system", "src", "models", "__fixtures__", name)
			_, err := os.Stat(candidate)
			return candidate, err == nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
