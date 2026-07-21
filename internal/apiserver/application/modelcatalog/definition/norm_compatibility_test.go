package definition_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

func TestCheckNormCompatibilityRejectsKindMismatch(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2}
	table := &norm.Norm{Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM, FormVariant: "standard", Factors: []norm.FactorTable{{FactorCode: "total"}}}
	issues := definition.CheckNormCompatibility(model, table, norm.Ref{FactorCode: "total", NormTableVersion: "spm-1"})
	if !hasIssueCode(issues, "norm.kind.mismatch") {
		t.Fatalf("issues = %#v, want norm.kind.mismatch", issues)
	}
}

func TestCheckNormCompatibilityRejectsFormVariantMismatch(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{
		Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2,
		DefinitionV2: &modeldefinition.Definition{Execution: modeldefinition.ExecutionSpec{Brief2: &modeldefinition.Brief2Spec{FormVariant: "parent"}}},
	}
	table := &norm.Norm{Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2, FormVariant: "teacher", Factors: []norm.FactorTable{{FactorCode: "gec"}}}
	issues := definition.CheckNormCompatibility(model, table, norm.Ref{FactorCode: "gec", NormTableVersion: "brief2-1"})
	if !hasIssueCode(issues, "norm.form_variant.mismatch") {
		t.Fatalf("issues = %#v, want norm.form_variant.mismatch", issues)
	}
}

func TestCheckNormCompatibilityAcceptsCognitiveSPM(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM}
	table := &norm.Norm{Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM, FormVariant: "standard", Factors: []norm.FactorTable{{FactorCode: "total"}}}
	issues := definition.CheckNormCompatibility(model, table, norm.Ref{FactorCode: "total", NormTableVersion: "spm-1"})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestCheckNormCompatibilityRejectsSPMTscoreBasis(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{
		Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM,
		DefinitionV2: &modeldefinition.Definition{
			Conclusions: []domain.Conclusion{
				domain.AbilityConclusion{FactorCode: "total", ScoreBasis: domain.ScoreBasisTScore, Primary: true},
			},
		},
	}
	table := &norm.Norm{
		Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM, FormVariant: "standard",
		Factors: []norm.FactorTable{{FactorCode: "total", Lookup: []norm.LookupEntry{{RawScoreMin: 1, RawScoreMax: 1, TScore: 50, Percentile: 50}}}},
	}
	issues := definition.CheckNormCompatibility(model, table, norm.Ref{FactorCode: "total", NormTableVersion: "spm-1"})
	if !hasIssueCode(issues, "norm.score_basis.unsupported") {
		t.Fatalf("issues = %#v, want norm.score_basis.unsupported", issues)
	}
}

func TestCheckNormCompatibilityRejectsMissingStandardScoreBasis(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{
		Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2,
		DefinitionV2: &modeldefinition.Definition{
			Conclusions: []domain.Conclusion{
				domain.NormConclusion{FactorCode: "gec", ScoreBasis: domain.ScoreBasisStandardScore, Primary: true},
			},
		},
	}
	table := &norm.Norm{
		Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2, FormVariant: "parent",
		Factors: []norm.FactorTable{{FactorCode: "gec", Lookup: []norm.LookupEntry{{RawScoreMin: 1, RawScoreMax: 1, TScore: 50, Percentile: 50}}}},
	}
	issues := definition.CheckNormCompatibility(model, table, norm.Ref{FactorCode: "gec", NormTableVersion: "brief2-1"})
	if !hasIssueCode(issues, "norm.score_basis.unsupported") {
		t.Fatalf("issues = %#v, want norm.score_basis.unsupported", issues)
	}
}

func TestCheckNormCompatibilityRequiresStandardScoreOnEveryReachableRow(t *testing.T) {
	t.Parallel()
	standard := 100.0
	model := &domain.AssessmentModel{
		Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2,
		DefinitionV2: &modeldefinition.Definition{Conclusions: []domain.Conclusion{
			domain.NormConclusion{FactorCode: "gec", ScoreBasis: domain.ScoreBasisStandardScore, Primary: true},
		}},
	}
	cases := []struct {
		name   string
		factor norm.FactorTable
		wantOK bool
	}{
		{
			name: "all lookup rows provide standard score",
			factor: norm.FactorTable{FactorCode: "gec", Lookup: []norm.LookupEntry{
				{RawScoreMin: 0, RawScoreMax: 9, StandardScore: &standard},
				{RawScoreMin: 10, RawScoreMax: 20, StandardScore: &standard},
			}},
			wantOK: true,
		},
		{
			name: "one lookup row missing standard score",
			factor: norm.FactorTable{FactorCode: "gec", Lookup: []norm.LookupEntry{
				{RawScoreMin: 0, RawScoreMax: 9, StandardScore: &standard},
				{RawScoreMin: 10, RawScoreMax: 20},
			}},
		},
		{
			name: "lookup plus band",
			factor: norm.FactorTable{FactorCode: "gec",
				Lookup: []norm.LookupEntry{{RawScoreMin: 0, RawScoreMax: 20, StandardScore: &standard}},
				Bands:  []norm.Band{{}},
			},
		},
		{name: "bands only", factor: norm.FactorTable{FactorCode: "gec", Bands: []norm.Band{{}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			table := &norm.Norm{Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2, FormVariant: "parent", Factors: []norm.FactorTable{tc.factor}}
			issues := definition.CheckNormCompatibility(model, table, norm.Ref{FactorCode: "gec", NormTableVersion: "brief2-1"})
			gotOK := !hasIssueCode(issues, "norm.score_basis.unsupported")
			if gotOK != tc.wantOK {
				t.Fatalf("issues = %#v, wantOK = %v", issues, tc.wantOK)
			}
		})
	}
}
