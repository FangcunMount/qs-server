package grpc

import (
	"testing"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	answerSheetApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	appQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
)

func TestRegistryGetRegisteredServicesReflectsTypedDeps(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(Deps{
		Survey: SurveyDeps{
			AnswerSheetSubmissionService: answerSheetApp.NewSubmissionService(nil, nil, nil, nil, nil),
			AnswerSheetManagementService: answerSheetApp.NewManagementService(nil, nil),
			QuestionnaireQueryService:    appQuestionnaire.NewQueryService(nil, nil, nil, nil),
		},
		AssessmentModelCatalog: AssessmentModelCatalogDeps{QueryService: modelcatalog.NewCatalogQueryService(modelcatalog.CatalogQueryDependencies{Authorizer: modelcatalog.SnapshotAuthorizer{}})},
	})

	got := registry.GetRegisteredServices()
	want := map[string]bool{
		"AnswerSheetService":            true,
		"QuestionnaireService":          true,
		"AssessmentModelCatalogService": true,
	}
	for _, name := range got {
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("GetRegisteredServices() missing %v, got %v", want, got)
	}
}
