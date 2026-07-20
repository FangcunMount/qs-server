package definition_test

import (
	"context"
	"strings"
	"testing"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestBehavioralValidateForPublishRejectsMissingNormSemantics(t *testing.T) {
	t.Parallel()

	handler := appdefinition.BehavioralRatingDefinitionHandler{NormRepo: behavioralNormRepoStub{table: &norm.Norm{
		TableVersion: "brief2-cn-2024",
		Factors:      []norm.FactorTable{{FactorCode: "bri", Lookup: []norm.LookupEntry{{RawScoreMin: 1, RawScoreMax: 3, TScore: 50, Percentile: 50}}}},
	}}}

	model := validBehavioralDraft()
	model.DefinitionV2.Calibration.NormRefs = nil
	issues := handler.ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "behavioral_rating.norm_refs.required") && !hasIssueCode(issues, "definition_v2.decision.invalid") {
		t.Fatalf("issues = %#v, want norm_refs/decision rejection", issues)
	}
}

func TestBehavioralValidateForPublishRejectsDefaultAlgorithm(t *testing.T) {
	t.Parallel()

	handler := appdefinition.BehavioralRatingDefinitionHandler{NormRepo: behavioralNormRepoStub{table: &norm.Norm{
		TableVersion: "brief2-cn-2024",
		Factors:      []norm.FactorTable{{FactorCode: "bri", Lookup: []norm.LookupEntry{{RawScoreMin: 1, RawScoreMax: 3, TScore: 50, Percentile: 50}}}},
	}}}
	model := validBehavioralDraft()
	model.Algorithm = domain.AlgorithmBehavioralRatingDefault
	issues := handler.ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "behavioral_rating.algorithm.required") {
		t.Fatalf("issues = %#v, want algorithm rejection", issues)
	}
}

func TestBehavioralValidateForPublishRejectsNormRefMissingInTable(t *testing.T) {
	t.Parallel()

	handler := appdefinition.BehavioralRatingDefinitionHandler{NormRepo: behavioralNormRepoStub{table: &norm.Norm{
		TableVersion: "brief2-cn-2024",
		Factors:      []norm.FactorTable{{FactorCode: "other", Lookup: []norm.LookupEntry{{RawScoreMin: 1, RawScoreMax: 3, TScore: 50, Percentile: 50}}}},
	}}}
	model := validBehavioralDraft()
	issues := handler.ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "behavioral_rating.norm_ref.factor.missing_in_table") {
		t.Fatalf("issues = %#v, want missing-in-table rejection", issues)
	}
}

func TestBehavioralValidateForPublishRejectsConclusionWithoutNormRef(t *testing.T) {
	t.Parallel()

	handler := appdefinition.BehavioralRatingDefinitionHandler{NormRepo: behavioralNormRepoStub{table: &norm.Norm{
		TableVersion: "brief2-cn-2024",
		Factors: []norm.FactorTable{
			{FactorCode: "bri", Lookup: []norm.LookupEntry{{RawScoreMin: 1, RawScoreMax: 3, TScore: 50, Percentile: 50}}},
			{FactorCode: "gec", Lookup: []norm.LookupEntry{{RawScoreMin: 1, RawScoreMax: 3, TScore: 50, Percentile: 50}}},
		},
	}}}
	model := validBehavioralDraft()
	model.DefinitionV2.Measure.Factors = append(model.DefinitionV2.Measure.Factors, factor.Factor{Code: "gec", Title: "GEC", Role: factor.FactorRoleDimension})
	model.DefinitionV2.Measure.Scoring = append(model.DefinitionV2.Measure.Scoring, factor.Scoring{
		FactorCode: "gec",
		Sources: []factor.ScoringSource{{
			Kind: factor.ScoringSourceQuestion, Code: "q2", Sign: 1, Weight: 1,
			ScoringMode: factor.QuestionScoringModeQuestionScore,
		}},
		Strategy:      factor.ScoringStrategySum,
		OptionScoring: factor.OptionScoringCompat,
	})
	model.DefinitionV2.Conclusions = append(model.DefinitionV2.Conclusions, domain.NormConclusion{FactorCode: "gec", ScoreBasis: domain.ScoreBasisTScore})
	issues := handler.ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "behavioral_rating.conclusion.norm_ref.missing") {
		t.Fatalf("issues = %#v, want conclusion/norm_ref mismatch", issues)
	}
}

func TestBehavioralValidateForPublishAcceptsBrief2(t *testing.T) {
	t.Parallel()

	handler := appdefinition.BehavioralRatingDefinitionHandler{NormRepo: behavioralNormRepoStub{table: &norm.Norm{
		TableVersion: "brief2-cn-2024",
		FormVariant:  "teacher",
		Factors:      []norm.FactorTable{{FactorCode: "bri", Lookup: []norm.LookupEntry{{RawScoreMin: 1, RawScoreMax: 3, TScore: 50, Percentile: 50}}}},
	}}}
	issues := handler.ValidateForPublish(context.Background(), validBehavioralDraft())
	for _, issue := range issues {
		if issue.Level == domain.ValidationLevelError {
			t.Fatalf("unexpected error issues = %#v", issues)
		}
	}
}

func TestBehavioralBuildSnapshotRejectsDefaultAlgorithm(t *testing.T) {
	t.Parallel()

	model := validBehavioralDraft()
	model.Algorithm = domain.AlgorithmBehavioralRatingDefault
	_, err := (appdefinition.BehavioralRatingDefinitionHandler{NormRepo: behavioralNormRepoStub{table: &norm.Norm{
		TableVersion: "brief2-cn-2024",
		Factors:      []norm.FactorTable{{FactorCode: "bri"}},
	}}}).BuildSnapshotPayload(context.Background(), model)
	if err == nil || !strings.Contains(err.Error(), "behavioral_rating_default") {
		t.Fatalf("err = %v, want behavioral_rating_default rejection", err)
	}
}

func validBehavioralDraft() *domain.AssessmentModel {
	return &domain.AssessmentModel{
		Kind:      domain.KindBehavioralRating,
		Algorithm: domain.AlgorithmBrief2,
		Code:      "brief2-valid",
		Title:     "Brief-2",
		Binding:   domain.QuestionnaireBinding{QuestionnaireCode: "Q", QuestionnaireVersion: "1"},
		Definition: domain.DefinitionPayload{
			Format: domain.PayloadFormatBehavioralRatingDefaultV1,
			Data:   []byte(`{"dimensions":[]}`),
		},
		DefinitionV2: &domain.Definition{
			Measure: modeldefinition.MeasureSpec{Factors: []factor.Factor{
				{Code: "bri", Title: "BRI", Role: factor.FactorRoleDimension},
			}, Scoring: []factor.Scoring{{
				FactorCode: "bri",
				Sources: []factor.ScoringSource{{
					Kind: factor.ScoringSourceQuestion, Code: "q1", Sign: 1, Weight: 1,
					ScoringMode: factor.QuestionScoringModeQuestionScore,
				}},
				Strategy:      factor.ScoringStrategySum,
				OptionScoring: factor.OptionScoringCompat,
			}}},
			Calibration: modeldefinition.Calibration{NormRefs: []norm.Ref{{FactorCode: "bri", NormTableVersion: "brief2-cn-2024"}}},
			Outcomes: []domain.Outcome{
				{Code: "normal", Title: "正常"},
				{Code: "elevated", Title: "升高"},
			},
			Conclusions: []domain.Conclusion{
				domain.NormConclusion{
					FactorCode: "bri", ScoreBasis: domain.ScoreBasisTScore, Primary: true,
					Rules: []domain.ScoreRangeOutcome{
						{MinScore: 0, MaxScore: 60, OutcomeCode: "normal", Level: "normal"},
						{MinScore: 60, MaxScore: 100, OutcomeCode: "elevated", Level: "elevated", MaxInclusive: true},
					},
				},
			},
			Execution: modeldefinition.ExecutionSpec{Brief2: &modeldefinition.Brief2Spec{PrimaryFactorCode: "bri"}},
		},
	}
}

func hasIssueCode(issues []domain.DomainValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

type behavioralNormRepoStub struct {
	table *norm.Norm
	err   error
}

func (s behavioralNormRepoStub) UpsertNorm(context.Context, *norm.Norm) error { return nil }
func (s behavioralNormRepoStub) ListNorms(context.Context, modelcatalogport.NormListFilter) ([]*norm.Norm, int64, error) {
	if s.table == nil {
		return nil, 0, s.err
	}
	return []*norm.Norm{s.table}, 1, s.err
}
func (s behavioralNormRepoStub) FindNorm(context.Context, string) (*norm.Norm, error) {
	return s.table, s.err
}

var _ modelcatalogport.NormRepository = behavioralNormRepoStub{}
