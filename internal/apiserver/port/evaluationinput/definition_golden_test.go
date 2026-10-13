package evaluationinput

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

type definitionGoldenFixture struct {
	Name          string                      `json:"name"`
	Definition    *modeldefinition.Definition `json:"definition"`
	Model         *ModelSnapshot              `json:"model"`
	Questionnaire *QuestionnaireSnapshot      `json:"questionnaire"`
	AnswerSheet   *AnswerSheetSnapshot        `json:"answersheet"`
	NormSubject   *NormSubjectSnapshot        `json:"norm_subject"`
	Identity      InputSnapshotIdentity       `json:"input_identity"`
	ReportInput   json.RawMessage             `json:"report_input"`
	FrozenAssets  json.RawMessage             `json:"frozen_assets"`
	FrozenPayload json.RawMessage             `json:"frozen_payload"`
}

func loadDefinitionGolden(t *testing.T, name string) definitionGoldenFixture {
	t.Helper()
	path := filepath.Join("..", "..", "domain", "modelcatalog", "definition", "testdata", "hash_v1", name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture definitionGoldenFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}
func (f definitionGoldenFixture) input() *InputSnapshot {
	return &InputSnapshot{Model: f.Model, Questionnaire: f.Questionnaire, AnswerSheet: f.AnswerSheet, NormSubject: f.NormSubject, DefinitionV2: f.Definition}
}
func compactGolden(t *testing.T, data []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := json.Compact(&out, data); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestDefinitionGoldenCorpusPreservesExecutionInputIdentity(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"scale", "typology", "behavioral_rating", "cognitive"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := loadDefinitionGolden(t, name)
			got, ok := NewInputSnapshotIdentity(fixture.input())
			if !ok {
				t.Fatal("complete corpus input did not derive an identity")
			}
			if got != fixture.Identity {
				t.Fatalf("retry input identity changed:\ngot %#v\nwant %#v", got, fixture.Identity)
			}
			// Stored derived copy is excluded from execution identity until the gated data cutover.
			fixture.Definition.InterpretationAssets.Outcomes[0].Summary = "stored mismatch"
			again, ok := NewInputSnapshotIdentity(fixture.input())
			if !ok || again != got {
				t.Fatal("derived asset edit changed the canonical execution identity")
			}
		})
	}
}

func TestDefinitionGoldenCorpusPreservesFrozenReportReplay(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"scale", "typology", "behavioral_rating", "cognitive"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := loadDefinitionGolden(t, name)
			frozen := append([]byte(nil), fixture.ReportInput...)
			// Later catalog edits must never alter this historical report's copy or route.
			fixture.Definition.Outcomes[0].Summary = "new catalog copy"
			fixture.Definition.InterpretationAssets.Outcomes[0].Summary = "new stored asset copy"
			restored, err := SnapshotFromReportInput(frozen, fixture.Model.ModelRef())
			if err != nil {
				t.Fatal(err)
			}
			assets, err := json.Marshal(restored.InterpretationAssets)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(assets, compactGolden(t, fixture.FrozenAssets)) {
				t.Fatal("historical frozen assets changed")
			}
			payload, err := json.Marshal(restored.ModelPayload)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(payload, compactGolden(t, fixture.FrozenPayload)) {
				t.Fatal("historical family payload or norm/profile/route semantics changed")
			}
			if !bytes.Equal(frozen, fixture.ReportInput) {
				t.Fatal("report replay modified durable input bytes")
			}
			if name == "typology" {
				if restored.TypologyRouting == nil || restored.TypologyRouting.TemplateVersion != "v1" {
					t.Fatal("historical typology template route was lost")
				}
				return
			}
			presentation, ok := restored.InterpretationAssets.FindOutcome("low")
			if !ok {
				t.Fatal("frozen outcome copy missing")
			}
			draft, err := scoring.BuildFactorScoringDraft(builder.NewDefaultReportBuilder(), scoring.FactorScoringReportInput{
				AssessmentID: report.ID(fixture.AnswerSheet.ID), TotalScore: 999,
				Scale:        &scoring.ReportModel{Code: fixture.Model.Code, Title: fixture.Model.Title, Assets: restored.InterpretationAssets, Factors: []scoring.FactorReportModel{{Code: "total", Title: "总分"}}},
				FactorScores: []scoring.FactorReportScore{{FactorCode: "total", RawScore: 999, IsTotalScore: true, Level: &report.ResultLevel{Code: "low"}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			content := draft.Content()
			if content.Conclusion != presentation.Summary || len(content.Dimensions) != 1 || content.Dimensions[0].Suggestion() != presentation.Description {
				t.Fatal("report rematched the score or replaced the frozen OutcomeCode presentation")
			}
		})
	}
}

func TestDefinitionGoldenStoredAssetsRemainPreferredBeforeDataCutover(t *testing.T) {
	t.Parallel()
	fixture := loadDefinitionGolden(t, "scale")
	fixture.Definition.Outcomes[0].Summary = "canonical differs from stored"
	input := fixture.input()
	AttachCanonicalDefinition(input, fixture.Definition)
	if input.InterpretationAssets == nil || input.InterpretationAssets.Outcomes[0].Summary != "冻结摘要<旧&v1>" {
		t.Fatal("foundation changed the existing stored-assets read preference")
	}
}
