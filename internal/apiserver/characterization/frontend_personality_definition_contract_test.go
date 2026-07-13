package characterization_test

import (
	"context"
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
	fixturePath, ok := frontendContractFixture()
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
		{Code: "q_drive", OptionCodes: []string{"low", "high"}},
		{Code: "q_care", OptionCodes: []string{"low", "high"}},
	}}
	if issues := modeltypology.ValidateRuntimeSpecForPublishWithContext(payload.Runtime, questionnaire, modeltypology.RuntimeSpecValidationContext{
		Algorithm: payload.Algorithm, Outcomes: payload.Outcomes,
	}); len(issues) != 0 {
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
	snapshot, err := (appdefinition.TypologyDefinitionHandler{}).BuildSnapshotPayload(context.Background(), model)
	if err != nil {
		t.Fatalf("build published snapshot: %v", err)
	}
	var frozen modeltypology.Payload
	if err := json.Unmarshal(snapshot.Payload, &frozen); err != nil {
		t.Fatalf("decode frozen snapshot: %v", err)
	}
	result, err := configured.NewEvaluator().Score(&frozen, &evalinput.AnswerSheet{
		QuestionnaireCode: "q_contract", QuestionnaireVersion: "v1",
		Answers: []evalinput.Answer{{QuestionCode: "q_drive", Value: "high"}, {QuestionCode: "q_care", Value: "low"}},
	})
	if err != nil {
		t.Fatalf("execute simulated answer sheet: %v", err)
	}
	if result.SelectedOutcome.Code != "drive" || len(result.Candidate.RankedFactors) != 2 {
		t.Fatalf("unexpected execution result: %#v", result)
	}
}

func frontendContractFixture() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if filepath.Base(dir) == "workspace" {
			candidate := filepath.Join(dir, "typescript", "github.com", "fangcun-mount", "qs-operating-system", "src", "models", "__fixtures__", "personalityDefinitionV2.contract.json")
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
