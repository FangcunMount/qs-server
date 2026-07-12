package definition_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestValidateSPMExecutionSpec(t *testing.T) {
	t.Parallel()

	def := definition.Definition{
		Measure: definition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Role: factor.FactorRoleTotal}}},
		Execution: definition.ExecutionSpec{SPM: &definition.SPMSpec{
			TimeLimitSeconds: 2400,
			TotalFactorCode:  "total",
			ItemSets:         []definition.SPMItemSet{{Code: "A", Items: []definition.SPMItem{{QuestionCode: "A1", CorrectOptionCode: "1"}}}},
		}},
	}
	if issues := definition.Validate(def); len(issues) != 0 {
		t.Fatalf("Validate() issues = %#v", issues)
	}
}

func TestValidateSPMExecutionSpecRejectsDuplicateQuestion(t *testing.T) {
	t.Parallel()

	def := definition.Definition{
		Measure: definition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Role: factor.FactorRoleTotal}}},
		Execution: definition.ExecutionSpec{SPM: &definition.SPMSpec{
			TimeLimitSeconds: 2400,
			TotalFactorCode:  "total",
			ItemSets: []definition.SPMItemSet{
				{Code: "A", Items: []definition.SPMItem{{QuestionCode: "A1", CorrectOptionCode: "1"}}},
				{Code: "B", Items: []definition.SPMItem{{QuestionCode: "A1", CorrectOptionCode: "2"}}},
			},
		}},
	}
	for _, issue := range definition.Validate(def) {
		if issue.Code == "spm.question.duplicate" {
			return
		}
	}
	t.Fatal("Validate() did not reject duplicate SPM question")
}
