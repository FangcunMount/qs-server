package legacyadapter

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

const defaultScaleVersion = "1.0.0"

func DefinitionPayloadFromScaleSnapshot(snapshot *scalesnapshot.ScaleSnapshot) (domain.DefinitionPayload, error) {
	if snapshot == nil {
		return domain.DefinitionPayload{}, fmt.Errorf("scale snapshot is nil")
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return domain.DefinitionPayload{}, fmt.Errorf("marshal scale definition: %w", err)
	}
	return domain.DefinitionPayload{
		Format: domain.PayloadFormatAssessmentScaleV1,
		Data:   payload,
	}, nil
}

func ScaleSnapshotFromDefinitionPayload(payload domain.DefinitionPayload) (*scalesnapshot.ScaleSnapshot, error) {
	if payload.Format != "" && payload.Format != domain.PayloadFormatAssessmentScaleV1 {
		return nil, fmt.Errorf("unsupported scale definition payload format %s", payload.Format)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("scale definition payload is empty")
	}
	return scalesnapshot.ParsePublishedPayload(payload.Data)
}

func AssessmentModelFromCreateDTO(dto shared.CreateScaleDTO, now time.Time) (*domain.AssessmentModel, error) {
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:           dto.Code,
		Kind:           domain.KindScale,
		Algorithm:      domain.AlgorithmScaleDefault,
		ProductChannel: domain.ProductChannelMedicalScale,
		Title:          dto.Title,
		Description:    dto.Description,
		Category:       dto.Category,
		Stages:         append([]string(nil), dto.Stages...),
		ApplicableAges: append([]string(nil), dto.ApplicableAges...),
		Reporters:      append([]string(nil), dto.Reporters...),
		Tags:           append([]string(nil), dto.Tags...),
		Now:            now,
	})
	if err != nil {
		return nil, err
	}
	if dto.QuestionnaireCode != "" || dto.QuestionnaireVersion != "" {
		if err := model.BindQuestionnaire(domain.QuestionnaireBinding{
			QuestionnaireCode:    dto.QuestionnaireCode,
			QuestionnaireVersion: dto.QuestionnaireVersion,
		}, now); err != nil {
			return nil, err
		}
	}
	snapshot := &scalesnapshot.ScaleSnapshot{
		Code:                 model.Code,
		ScaleVersion:         defaultScaleVersion,
		Title:                model.Title,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status:               string(model.Status),
	}
	if err := applyScaleSnapshotEnvelope(model, snapshot); err != nil {
		return nil, err
	}
	return model, nil
}

func ScaleResultFromAssessmentModel(model *domain.AssessmentModel) (*shared.ScaleResult, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	snapshot, err := ScaleSnapshotFromDefinitionPayload(model.Definition)
	if err != nil {
		return nil, err
	}
	return ScaleResultFromSnapshot(model, snapshot), nil
}

func ScaleResultFromSnapshot(model *domain.AssessmentModel, snapshot *scalesnapshot.ScaleSnapshot) *shared.ScaleResult {
	if model == nil || snapshot == nil {
		return nil
	}
	result := &shared.ScaleResult{
		Code:                 model.Code,
		ScaleVersion:         snapshot.ScaleVersion,
		Title:                model.Title,
		Description:          model.Description,
		Category:             model.Category,
		Stages:               append([]string(nil), model.Stages...),
		ApplicableAges:       append([]string(nil), model.ApplicableAges...),
		Reporters:            append([]string(nil), model.Reporters...),
		Tags:                 append([]string(nil), model.Tags...),
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Status:               string(model.Status),
		Factors:              make([]shared.FactorResult, 0, len(snapshot.Factors)),
		CreatedAt:            model.CreatedAt,
		UpdatedAt:            model.UpdatedAt,
	}
	for _, factor := range snapshot.Factors {
		result.Factors = append(result.Factors, factorResultFromSnapshot(factor))
	}
	return result
}

func ScaleResultFromPublishedModel(snapshot *port.PublishedModel) (*shared.ScaleResult, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("published scale snapshot is nil")
	}
	if snapshot.Kind != "" && snapshot.Kind != domain.KindScale {
		return nil, fmt.Errorf("published model kind %s is not scale", snapshot.Kind)
	}
	if snapshot.DefinitionV2 == nil {
		return nil, fmt.Errorf("published scale definition_v2 is required")
	}
	legacy, _ := port.LegacyScaleBindingFromPublished(snapshot)
	version := legacy.ScaleVersion
	if version == "" {
		version = snapshot.Version
	}
	scaleSnapshot := scalesnapshot.ScaleSnapshotFromDefinition(scalesnapshot.ExecutionEnvelope{
		Code:                 snapshot.Code,
		ScaleVersion:         version,
		Title:                snapshot.Title,
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Status:               snapshot.Status,
	}, snapshot.DefinitionV2)
	if scaleSnapshot == nil {
		return nil, fmt.Errorf("published scale definition_v2 cannot produce runtime snapshot")
	}
	model := &domain.AssessmentModel{
		Code:           snapshot.Code,
		Kind:           domain.KindScale,
		Algorithm:      snapshot.Algorithm,
		ProductChannel: snapshot.ProductChannel,
		Title:          snapshot.Title,
		Description:    snapshot.Description,
		Category:       snapshot.Category,
		Stages:         append([]string(nil), snapshot.Stages...),
		ApplicableAges: append([]string(nil), snapshot.ApplicableAges...),
		Reporters:      append([]string(nil), snapshot.Reporters...),
		Tags:           append([]string(nil), snapshot.Tags...),
		Status:         domain.ModelStatus(snapshot.Status),
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    snapshot.QuestionnaireCode,
			QuestionnaireVersion: snapshot.QuestionnaireVersion,
		},
		DefinitionV2: snapshot.DefinitionV2,
	}
	return ScaleResultFromSnapshot(model, scaleSnapshot), nil
}

func factorResultFromSnapshot(snapshot scalesnapshot.FactorSnapshot) shared.FactorResult {
	result := shared.FactorResult{
		Code:            snapshot.Code,
		Title:           snapshot.Title,
		IsTotalScore:    snapshot.IsTotalScore,
		QuestionCodes:   append([]string(nil), snapshot.QuestionCodes...),
		ScoringStrategy: snapshot.ScoringStrategy,
		ScoringParams:   scoringParamsResultMap(snapshot.ScoringParams, snapshot.ScoringStrategy),
		MaxScore:        cloneFloat64(snapshot.MaxScore),
		RiskLevel:       "none",
		InterpretRules:  make([]shared.InterpretRuleResult, 0, len(snapshot.InterpretRules)),
	}
	for i, rule := range snapshot.InterpretRules {
		result.InterpretRules = append(result.InterpretRules, shared.InterpretRuleResult{
			MinScore:   rule.Min,
			MaxScore:   rule.Max,
			RiskLevel:  rule.RiskLevel,
			Conclusion: rule.Conclusion,
			Suggestion: rule.Suggestion,
		})
		if i == 0 {
			result.RiskLevel = rule.RiskLevel
		}
	}
	return result
}

func scoringParamsResultMap(params scalesnapshot.ScoringParamsSnapshot, strategy string) map[string]interface{} {
	result := make(map[string]interface{})
	if strategy == "cnt" && len(params.CntOptionContents) > 0 {
		result["cnt_option_contents"] = append([]string(nil), params.CntOptionContents...)
	}
	return result
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// ForkAssessmentModelDraftFromPublished forks a published scale head into a draft
// working version while keeping the active published runtime snapshot unchanged.
func ForkAssessmentModelDraftFromPublished(model *domain.AssessmentModel, now time.Time) error {
	if model == nil || !model.IsPublished() {
		return nil
	}
	snapshot, err := scaleSnapshotEnvelope(model)
	if err != nil {
		return err
	}
	currentVersion := snapshot.ScaleVersion
	if currentVersion == "" {
		currentVersion = defaultScaleVersion
	}
	snapshot.ScaleVersion = incrementPatchVersion(currentVersion)
	snapshot.Status = "draft"
	if err := applyScaleSnapshotEnvelope(model, snapshot); err != nil {
		return err
	}
	return model.ForkDraftFromPublished(now)
}

// SyncScaleMetadataInModel projects editable metadata onto the scale payload envelope.
func SyncScaleMetadataInModel(model *domain.AssessmentModel) error {
	if model == nil || model.Definition.IsEmpty() {
		return nil
	}
	snapshot, err := ScaleSnapshotFromDefinitionPayload(model.Definition)
	if err != nil {
		return err
	}
	snapshot.Code = model.Code
	snapshot.Title = model.Title
	snapshot.QuestionnaireCode = model.Binding.QuestionnaireCode
	snapshot.QuestionnaireVersion = model.Binding.QuestionnaireVersion
	snapshot.Status = string(model.Status)
	return applyScaleSnapshotEnvelope(model, snapshot)
}

func scaleSnapshotEnvelope(model *domain.AssessmentModel) (*scalesnapshot.ScaleSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	if !model.Definition.IsEmpty() {
		return ScaleSnapshotFromDefinitionPayload(model.Definition)
	}
	return &scalesnapshot.ScaleSnapshot{
		Code:                 model.Code,
		Title:                model.Title,
		ScaleVersion:         defaultScaleVersion,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status:               string(model.Status),
	}, nil
}

func applyScaleSnapshotEnvelope(model *domain.AssessmentModel, snapshot *scalesnapshot.ScaleSnapshot) error {
	if model == nil || snapshot == nil {
		return fmt.Errorf("assessment model or scale snapshot is nil")
	}
	payload, err := DefinitionPayloadFromScaleSnapshot(snapshot)
	if err != nil {
		return err
	}
	model.Definition = payload
	model.DefinitionV2 = scalesnapshot.DefinitionFromScaleSnapshot(snapshot)
	return nil
}

func incrementPatchVersion(version string) string {
	raw := version
	if raw == "" {
		raw = defaultScaleVersion
	}
	prefix := ""
	if strings.HasPrefix(raw, "v") {
		prefix = "v"
		raw = strings.TrimPrefix(raw, "v")
	}
	parts := strings.Split(raw, ".")
	switch len(parts) {
	case 0:
		return prefix + "1.0.1"
	case 1:
		return prefix + parts[0] + ".0.1"
	case 2:
		return prefix + parts[0] + "." + parts[1] + ".1"
	default:
		patch, err := strconv.Atoi(parts[2])
		if err != nil {
			patch = 0
		}
		patch++
		return prefix + parts[0] + "." + parts[1] + "." + strconv.Itoa(patch)
	}
}
