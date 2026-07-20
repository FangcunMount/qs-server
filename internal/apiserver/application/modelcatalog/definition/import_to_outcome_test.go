package definition_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/task_performance"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	portevaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	cognitivepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

func TestImportToOutcomeCognitiveSPMRetainsNormReference(t *testing.T) {
	t.Parallel()
	standard := 108.0
	table := &norm.Norm{
		TableVersion: "spm-cn-2024", FormVariant: "standard",
		Kind: identity.KindCognitive, Algorithm: identity.AlgorithmSPM,
		Factors: []norm.FactorTable{{
			FactorCode: "total",
			Lookup: []norm.LookupEntry{{
				RawScoreMin: 1, RawScoreMax: 1, TScore: 50, Percentile: 70, StandardScore: &standard,
				MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female",
			}},
		}},
	}
	if err := norm.ValidateImport(table); err != nil {
		t.Fatalf("ValidateImport: %v", err)
	}
	model := &domain.AssessmentModel{
		Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM,
		DefinitionV2: &modeldefinition.Definition{
			Conclusions: []domain.Conclusion{
				domain.AbilityConclusion{FactorCode: "total", ScoreBasis: domain.ScoreBasisPercentile, Primary: true},
			},
		},
	}
	if issues := definition.CheckNormCompatibility(model, table, norm.Ref{FactorCode: "total", NormTableVersion: table.TableVersion}); len(issues) != 0 {
		t.Fatalf("CheckNormCompatibility issues = %#v", issues)
	}
	runtimeTables := cognitivepayload.NormTablesFromCatalog(table)
	if runtimeTables == nil || runtimeTables.NormTableVersion != table.TableVersion {
		t.Fatalf("NormTablesFromCatalog = %#v", runtimeTables)
	}
	snapshot := &cognitivepayload.Snapshot{
		Code: "SPM", Version: "1", Title: "SPM",
		SPM: &cognitivepayload.SPMSpec{
			TotalFactorCode: "total",
			ItemSets:        []cognitivepayload.SPMItemSet{{Code: "A", Items: []cognitivepayload.SPMItem{{QuestionCode: "A1", CorrectOptionCode: "1"}}}},
			NormTables:      runtimeTables,
		},
	}
	input := &portevaluationinput.InputSnapshot{
		AnswerSheet: &portevaluationinput.AnswerSheetSnapshot{Answers: []portevaluationinput.AnswerSnapshot{{QuestionCode: "A1", Value: "1"}}},
		NormSubject: &portevaluationinput.NormSubjectSnapshot{AgeMonths: 72, Gender: "female"},
	}
	got, err := task_performance.CalculateSPM(input, snapshot)
	if err != nil {
		t.Fatalf("CalculateSPM: %v", err)
	}
	total := got.Dimensions[len(got.Dimensions)-1]
	if total.NormReference == nil || total.NormReference.TableVersion != "spm-cn-2024" {
		t.Fatalf("NormReference = %#v", total.NormReference)
	}
}

func TestImportToPublishBehavioralRejectsMissingStandardScoreBasis(t *testing.T) {
	t.Parallel()
	table := &norm.Norm{
		TableVersion: "brief2-parent-2026", FormVariant: "parent",
		Kind: identity.KindBehavioralRating, Algorithm: identity.AlgorithmBrief2,
		Factors: []norm.FactorTable{{
			FactorCode: "gec",
			Lookup:     []norm.LookupEntry{{RawScoreMin: 10, RawScoreMax: 10, TScore: 55, Percentile: 69}},
		}},
	}
	if err := norm.ValidateImport(table); err != nil {
		t.Fatalf("ValidateImport: %v", err)
	}
	model := &domain.AssessmentModel{
		Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2,
		DefinitionV2: &modeldefinition.Definition{
			Conclusions: []domain.Conclusion{
				domain.NormConclusion{FactorCode: "gec", ScoreBasis: domain.ScoreBasisStandardScore, Primary: true},
			},
		},
	}
	issues := definition.CheckNormCompatibility(model, table, norm.Ref{FactorCode: "gec", NormTableVersion: table.TableVersion})
	if !hasIssueCode(issues, "norm.score_basis.unsupported") {
		t.Fatalf("issues = %#v, want unsupported standard_score", issues)
	}
}
