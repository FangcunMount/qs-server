package service

import (
	"testing"

	appquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
)

func TestToProtoQuestionnaireCarriesShowControllerForSubmissionPreflight(t *testing.T) {
	result, err := (&QuestionnaireService{}).toProtoQuestionnaire(&appquestionnaire.QuestionnaireResult{
		Code: "Q", Version: "1", Questions: []appquestionnaire.QuestionResult{{
			Code: "follow", Type: "Text", ShowController: &appquestionnaire.ShowControllerResult{
				Rule: "and", Conditions: []appquestionnaire.ShowControllerConditionResult{{QuestionCode: "trigger", OptionCodes: []string{"yes"}}},
			},
		}},
	})
	if err != nil {
		t.Fatalf("toProtoQuestionnaire() error = %v", err)
	}
	got := result.Questions[0].GetShowController()
	if got == nil || got.GetRule() != "and" || len(got.GetConditions()) != 1 || got.GetConditions()[0].GetQuestionCode() != "trigger" {
		t.Fatalf("show controller = %+v", got)
	}
}
