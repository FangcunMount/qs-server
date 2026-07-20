package questionnaireref_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/questionnaireref"
)

func TestValidateRefsRequiresExistingQuestionAndOption(t *testing.T) {
	t.Parallel()

	idx := questionnaireref.NewIndex([]questionnaireref.Question{{
		Code: "Q1", Type: "single_choice", OptionCodes: []string{"A", "B"},
	}})

	issues := idx.ValidateRefs([]questionnaireref.Ref{
		{Field: "scoring[total].sources", QuestionCode: "Q1"},
		{Field: "scoring[total].sources", QuestionCode: "Q1", OptionCode: "A"},
		{Field: "scoring[total].sources", QuestionCode: "MISSING"},
		{Field: "scoring[total].sources", QuestionCode: "Q1", OptionCode: "Z"},
		{Field: "scoring[total].sources", QuestionCode: ""},
	})
	if !hasIssueCode(issues, "question_mapping.question_not_found") {
		t.Fatalf("issues = %#v, want question_not_found", issues)
	}
	if !hasIssueCode(issues, "question_mapping.option_not_found") {
		t.Fatalf("issues = %#v, want option_not_found", issues)
	}
	if !hasIssueCode(issues, "question_mapping.question_code.required") {
		t.Fatalf("issues = %#v, want question_code.required", issues)
	}
	if len(issues) != 3 {
		t.Fatalf("issue count = %d, want 3 blocking refs: %#v", len(issues), issues)
	}
}

func hasIssueCode(issues []binding.DomainValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
