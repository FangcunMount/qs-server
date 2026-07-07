package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestValidateDefinitionBodyForPublishAcceptsFlatModel(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateDefinitionBodyForPublish(factor.DefinitionBody{
		Dimensions: []factor.DimensionRule{{
			Code: "total", Title: "总分", ScoringStrategy: "sum", QuestionCodes: []string{"q1"},
		}},
		InterpretRules: []factor.InterpretRule{{
			DimensionCode: "total",
			Ranges:        []factor.ScoreRangeRule{{MinScore: 0, MaxScore: 10, Conclusion: "ok"}},
		}},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestValidateDefinitionBodyForPublishRejectsInvalidHierarchy(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateDefinitionBodyForPublish(factor.DefinitionBody{
		Dimensions: []factor.DimensionRule{{
			Code: "bri", Role: string(factor.FactorRoleIndex), ParentCode: "gec",
		}},
	})
	if len(issues) == 0 {
		t.Fatal("expected hierarchy validation issues")
	}
}

func TestValidateDefinitionBodyForPublishRejectsUnknownInterpretRuleDimension(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateDefinitionBodyForPublish(factor.DefinitionBody{
		Dimensions: []factor.DimensionRule{{
			Code: "total", Title: "总分", ScoringStrategy: "sum",
		}},
		InterpretRules: []factor.InterpretRule{{
			DimensionCode: "missing",
			Ranges:        []factor.ScoreRangeRule{{MinScore: 0, MaxScore: 1}},
		}},
	})
	if len(issues) == 0 {
		t.Fatal("expected interpret_rules.dimension_code.not_found issue")
	}
}

func TestValidateDefinitionBodyJSONForPublish(t *testing.T) {
	t.Parallel()

	issues, err := factor.ValidateDefinitionBodyJSONForPublish([]byte(`{"dimensions":[]}`))
	if err != nil {
		t.Fatalf("ValidateDefinitionBodyJSONForPublish: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected dimensions.required issue")
	}
}
