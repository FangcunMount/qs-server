package legacyadapter

import (
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func TestAssessmentModelFromCreateDTOUsesScalePayloadContract(t *testing.T) {
	t.Parallel()

	model, err := AssessmentModelFromCreateDTO(shared.CreateScaleDTO{
		Code:                 "SCL_DTO",
		Title:                "DTO Scale",
		Description:          "created from route dto",
		Category:             "adhd",
		Stages:               []string{"deep_assessment"},
		ApplicableAges:       []string{"school_child"},
		Reporters:            []string{"parent"},
		Tags:                 []string{"screening"},
		QuestionnaireCode:    "Q_DTO",
		QuestionnaireVersion: "1.0.0",
	}, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromCreateDTO: %v", err)
	}
	if model.Code != "SCL_DTO" || model.Title != "DTO Scale" || model.Binding.QuestionnaireCode != "Q_DTO" {
		t.Fatalf("model = %#v", model)
	}
	if !reflect.DeepEqual(model.Stages, []string{"deep_assessment"}) ||
		!reflect.DeepEqual(model.ApplicableAges, []string{"school_child"}) ||
		!reflect.DeepEqual(model.Reporters, []string{"parent"}) {
		t.Fatalf("model audience metadata stages=%v ages=%v reporters=%v",
			model.Stages, model.ApplicableAges, model.Reporters)
	}
	snapshot, err := ScaleSnapshotFromDefinitionPayload(model.Definition)
	if err != nil {
		t.Fatalf("ScaleSnapshotFromDefinitionPayload: %v", err)
	}
	if snapshot.Code != "SCL_DTO" || snapshot.ScaleVersion != defaultScaleVersion ||
		snapshot.QuestionnaireVersion != "1.0.0" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestScaleResultFromAssessmentModelProjectsResponseShape(t *testing.T) {
	t.Parallel()

	model := newAdapterScaleModel(t)
	result, err := ScaleResultFromAssessmentModel(model)
	if err != nil {
		t.Fatalf("ScaleResultFromAssessmentModel: %v", err)
	}
	if result.Code != "SCL_ADAPTER" || result.ScaleVersion != "1.0.0" ||
		result.Category != "adhd" || result.Status != "published" ||
		!reflect.DeepEqual(result.Stages, []string{"deep_assessment"}) ||
		!reflect.DeepEqual(result.ApplicableAges, []string{"school_child"}) ||
		!reflect.DeepEqual(result.Reporters, []string{"parent"}) ||
		!reflect.DeepEqual(result.Tags, []string{"screening", "clinical"}) {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Factors) != 2 {
		t.Fatalf("factor count = %d", len(result.Factors))
	}
	cntFactor := result.Factors[1]
	if cntFactor.ScoringStrategy != "cnt" || cntFactor.RiskLevel != "low" {
		t.Fatalf("factor result = %#v", cntFactor)
	}
	if got := cntFactor.ScoringParams["cnt_option_contents"]; !reflect.DeepEqual(got, []string{"yes", "often"}) {
		t.Fatalf("cnt params = %#v", got)
	}
}

func TestScaleResultFromPublishedModelProjectsResponseShape(t *testing.T) {
	t.Parallel()

	model := newAdapterScaleModel(t)
	published := &port.PublishedModel{
		Code:                 model.Code,
		Kind:                 domain.KindScale,
		Algorithm:            model.Algorithm,
		ProductChannel:       model.ProductChannel,
		Title:                model.Title,
		Description:          model.Description,
		Category:             model.Category,
		Stages:               append([]string(nil), model.Stages...),
		ApplicableAges:       append([]string(nil), model.ApplicableAges...),
		Reporters:            append([]string(nil), model.Reporters...),
		Tags:                 append([]string(nil), model.Tags...),
		Status:               string(model.Status),
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		PayloadFormat:        model.Definition.Format,
		Payload:              []byte(`not-json`),
		DefinitionV2:         model.DefinitionV2,
	}
	port.SetLegacyScaleBinding(published, port.LegacyScaleBinding{ScaleVersion: "1.0.0"})
	result, err := ScaleResultFromPublishedModel(published)
	if err != nil {
		t.Fatalf("ScaleResultFromPublishedModel: %v", err)
	}
	if result.Code != model.Code || result.Status != "published" || len(result.Factors) != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestForkAssessmentModelDraftFromPublishedIncrementsSnapshotVersion(t *testing.T) {
	t.Parallel()

	model := newAdapterScaleModel(t)
	if err := ForkAssessmentModelDraftFromPublished(model, time.Date(2026, 7, 9, 13, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ForkAssessmentModelDraftFromPublished: %v", err)
	}
	snapshot, err := ScaleSnapshotFromDefinitionPayload(model.Definition)
	if err != nil {
		t.Fatalf("ScaleSnapshotFromDefinitionPayload: %v", err)
	}
	if model.Status != domain.ModelStatusDraft || snapshot.ScaleVersion != "1.0.1" || snapshot.Status != "draft" {
		t.Fatalf("forked model status=%s snapshot=%#v", model.Status, snapshot)
	}
}

func TestSyncScaleMetadataInModelUpdatesPayloadEnvelope(t *testing.T) {
	t.Parallel()

	model := newAdapterScaleModel(t)
	model.Title = "Renamed"
	model.Binding.QuestionnaireCode = "Q_RENAMED"
	model.Binding.QuestionnaireVersion = "2.0.0"
	if err := SyncScaleMetadataInModel(model); err != nil {
		t.Fatalf("SyncScaleMetadataInModel: %v", err)
	}
	snapshot, err := ScaleSnapshotFromDefinitionPayload(model.Definition)
	if err != nil {
		t.Fatalf("ScaleSnapshotFromDefinitionPayload: %v", err)
	}
	if snapshot.Title != "Renamed" || snapshot.QuestionnaireCode != "Q_RENAMED" || snapshot.QuestionnaireVersion != "2.0.0" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func newAdapterScaleModel(t *testing.T) *domain.AssessmentModel {
	t.Helper()

	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	model, err := AssessmentModelFromCreateDTO(shared.CreateScaleDTO{
		Code:                 "SCL_ADAPTER",
		Title:                "Adapter Scale",
		Description:          "scale definition",
		Category:             "adhd",
		Stages:               []string{"deep_assessment"},
		ApplicableAges:       []string{"school_child"},
		Reporters:            []string{"parent"},
		Tags:                 []string{"screening", "clinical"},
		QuestionnaireCode:    "Q_ADAPTER",
		QuestionnaireVersion: "1.0.0",
	}, now)
	if err != nil {
		t.Fatalf("AssessmentModelFromCreateDTO: %v", err)
	}
	maxScore := 10.0
	snapshot := &scalesnapshot.ScaleSnapshot{
		Code:                 model.Code,
		ScaleVersion:         "1.0.0",
		Title:                model.Title,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status:               "published",
		Factors: []scalesnapshot.FactorSnapshot{{
			Code:            "TOTAL",
			Title:           "Total",
			IsTotalScore:    true,
			ScoringStrategy: "sum",
			MaxScore:        &maxScore,
			InterpretRules: []scalesnapshot.InterpretRuleSnapshot{{
				Min: 0, Max: 10, RiskLevel: "none", Conclusion: "none", Suggestion: "keep",
			}},
		}, {
			Code:            "CNT",
			Title:           "Count Factor",
			QuestionCodes:   []string{"Q1", "Q2"},
			ScoringStrategy: "cnt",
			ScoringParams:   scalesnapshot.ScoringParamsSnapshot{CntOptionContents: []string{"yes", "often"}},
			MaxScore:        &maxScore,
			InterpretRules: []scalesnapshot.InterpretRuleSnapshot{
				{Min: 0, Max: 5, RiskLevel: "low", Conclusion: "low", Suggestion: "watch"},
				{Min: 5, Max: 10, RiskLevel: "high", Conclusion: "high", Suggestion: "act"},
			},
		}},
	}
	if err := applyScaleSnapshotEnvelope(model, snapshot); err != nil {
		t.Fatalf("applyScaleSnapshotEnvelope: %v", err)
	}
	if err := model.MarkPublished(now); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}
	return model
}
