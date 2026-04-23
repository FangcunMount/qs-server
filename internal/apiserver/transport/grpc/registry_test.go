package grpc

import (
	"testing"

	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	answerSheetApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	appQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
)

func TestRegistryGetRegisteredServicesReflectsTypedDeps(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(Deps{
		Survey: SurveyDeps{
			AnswerSheetSubmissionService: answerSheetApp.NewSubmissionService(nil, nil, nil, nil),
			AnswerSheetManagementService: answerSheetApp.NewManagementService(nil),
			QuestionnaireQueryService:    appQuestionnaire.NewQueryService(nil, nil, nil),
		},
		Scale: ScaleDeps{
			QueryService:    scaleApp.NewQueryService(nil, nil, nil, nil),
			CategoryService: scaleApp.NewCategoryService(),
		},
	})

	got := registry.GetRegisteredServices()
	want := map[string]bool{
		"AnswerSheetService":   true,
		"QuestionnaireService": true,
		"ScaleService":         true,
	}
	for _, name := range got {
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("GetRegisteredServices() missing %v, got %v", want, got)
	}
}
