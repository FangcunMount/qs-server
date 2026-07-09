package legacyadapter

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// AssessmentModelFromMedicalScale adapts the legacy medical scale aggregate to
// the target AssessmentModel write model. It preserves the scale execution
// definition as assessment_scale_v1 JSON so published snapshot payloads remain
// byte-compatible with the legacy path.
func AssessmentModelFromMedicalScale(scale *scaledefinition.MedicalScale, now time.Time) (*domain.AssessmentModel, error) {
	if scale == nil {
		return nil, fmt.Errorf("medical scale is nil")
	}
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:           scale.GetCode().String(),
		Kind:           domain.KindScale,
		Algorithm:      domain.AlgorithmScaleDefault,
		ProductChannel: domain.ProductChannelMedicalScale,
		Title:          scale.GetTitle(),
		Description:    scale.GetDescription(),
		Category:       scale.GetCategory().String(),
		Stages:         stringsFromStages(scale.GetStages()),
		ApplicableAges: stringsFromApplicableAges(scale.GetApplicableAges()),
		Reporters:      stringsFromReporters(scale.GetReporters()),
		Tags:           stringsFromTags(scale.GetTags()),
		Now:            now,
	})
	if err != nil {
		return nil, err
	}
	if !scale.GetQuestionnaireCode().IsEmpty() && scale.GetQuestionnaireVersion() != "" {
		if err := model.BindQuestionnaire(domain.QuestionnaireBinding{
			QuestionnaireCode:    scale.GetQuestionnaireCode().String(),
			QuestionnaireVersion: scale.GetQuestionnaireVersion(),
		}, now); err != nil {
			return nil, err
		}
	}
	snapshot := ScaleSnapshotFromMedicalScale(scale)
	payload, err := DefinitionPayloadFromScaleSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	if err := model.UpdateDefinitionWithV2(payload, scalesnapshot.DefinitionFromScaleSnapshot(snapshot), now); err != nil {
		return nil, err
	}
	switch scale.GetStatus() {
	case scaledefinition.StatusPublished:
		if err := model.MarkPublished(now); err != nil {
			return nil, err
		}
	case scaledefinition.StatusArchived:
		if err := model.MarkArchived(now); err != nil {
			return nil, err
		}
	}
	return model, nil
}

func DefinitionPayloadFromMedicalScale(scale *scaledefinition.MedicalScale) (domain.DefinitionPayload, error) {
	return DefinitionPayloadFromScaleSnapshot(ScaleSnapshotFromMedicalScale(scale))
}

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

func ScaleSnapshotFromMedicalScale(scale *scaledefinition.MedicalScale) *scalesnapshot.ScaleSnapshot {
	if scale == nil {
		return nil
	}
	factors := make([]scalesnapshot.FactorSnapshot, 0, len(scale.FactorSnapshots()))
	for _, snapshot := range scale.FactorSnapshots() {
		factors = append(factors, ScaleFactorSnapshotFromMedicalScale(snapshot))
	}
	return &scalesnapshot.ScaleSnapshot{
		ID:                   scale.GetID().Uint64(),
		Code:                 scale.GetCode().String(),
		ScaleVersion:         scale.GetScaleVersion(),
		Title:                scale.GetTitle(),
		QuestionnaireCode:    scale.GetQuestionnaireCode().String(),
		QuestionnaireVersion: scale.GetQuestionnaireVersion(),
		Status:               scale.GetStatus().String(),
		Factors:              factors,
	}
}

func ScaleFactorSnapshotFromMedicalScale(snapshot scaledefinition.FactorSnapshot) scalesnapshot.FactorSnapshot {
	questionCodes := make([]string, 0, len(snapshot.QuestionCodes))
	for _, code := range snapshot.QuestionCodes {
		questionCodes = append(questionCodes, code.String())
	}
	rules := make([]scalesnapshot.InterpretRuleSnapshot, 0, len(snapshot.InterpretRules))
	for _, rule := range snapshot.InterpretRules {
		rules = append(rules, scalesnapshot.InterpretRuleSnapshot{
			Min:        rule.GetScoreRange().Min(),
			Max:        rule.GetScoreRange().Max(),
			RiskLevel:  rule.GetRiskLevel().String(),
			Conclusion: rule.GetConclusion(),
			Suggestion: rule.GetSuggestion(),
		})
	}
	cntContents := []string(nil)
	if snapshot.ScoringParams != nil {
		cntContents = append([]string(nil), snapshot.ScoringParams.GetCntOptionContents()...)
	}
	return scalesnapshot.FactorSnapshot{
		Code:            snapshot.Code.String(),
		Title:           snapshot.Title,
		IsTotalScore:    snapshot.IsTotalScore,
		QuestionCodes:   questionCodes,
		ScoringStrategy: snapshot.ScoringStrategy.String(),
		ScoringParams: scalesnapshot.ScoringParamsSnapshot{
			CntOptionContents: cntContents,
		},
		MaxScore:       cloneFloat64(snapshot.MaxScore),
		InterpretRules: rules,
	}
}

func AssessmentModelFromCreateDTO(dto shared.CreateScaleDTO, now time.Time) (*domain.AssessmentModel, error) {
	scale, err := MedicalScaleFromCreateDTO(dto)
	if err != nil {
		return nil, err
	}
	return AssessmentModelFromMedicalScale(scale, now)
}

func MedicalScaleFromCreateDTO(dto shared.CreateScaleDTO) (*scaledefinition.MedicalScale, error) {
	opts := []scaledefinition.MedicalScaleOption{
		scaledefinition.WithDescription(dto.Description),
		scaledefinition.WithCategory(scaledefinition.NewCategory(dto.Category)),
		scaledefinition.WithStages(stagesFromStrings(dto.Stages)),
		scaledefinition.WithApplicableAges(applicableAgesFromStrings(dto.ApplicableAges)),
		scaledefinition.WithReporters(reportersFromStrings(dto.Reporters)),
		scaledefinition.WithTags(tagsFromStrings(dto.Tags)),
	}
	if dto.QuestionnaireCode != "" || dto.QuestionnaireVersion != "" {
		opts = append(opts, scaledefinition.WithQuestionnaire(meta.NewCode(dto.QuestionnaireCode), dto.QuestionnaireVersion))
	}
	return scaledefinition.NewMedicalScale(meta.NewCode(dto.Code), dto.Title, opts...)
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
	scaleSnapshot, err := ScaleSnapshotFromDefinitionPayload(domain.DefinitionPayload{
		Format: snapshot.PayloadFormat,
		Data:   snapshot.Payload,
	})
	if err != nil {
		return nil, err
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
		Definition: domain.DefinitionPayload{
			Format: snapshot.PayloadFormat,
			Data:   append([]byte(nil), snapshot.Payload...),
		},
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
	if strategy == scaledefinition.ScoringStrategyCnt.String() && len(params.CntOptionContents) > 0 {
		result["cnt_option_contents"] = append([]string(nil), params.CntOptionContents...)
	}
	return result
}

func tagsFromStrings(values []string) []scaledefinition.Tag {
	out := make([]scaledefinition.Tag, 0, len(values))
	for _, value := range values {
		out = append(out, scaledefinition.NewTag(value))
	}
	return out
}

func stringsFromTags(values []scaledefinition.Tag) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.String())
	}
	return out
}

func stringsFromStages(values []scaledefinition.Stage) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.String())
	}
	return out
}

func stringsFromApplicableAges(values []scaledefinition.ApplicableAge) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.String())
	}
	return out
}

func stringsFromReporters(values []scaledefinition.Reporter) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.String())
	}
	return out
}

func stagesFromStrings(values []string) []scaledefinition.Stage {
	out := make([]scaledefinition.Stage, 0, len(values))
	for _, value := range values {
		out = append(out, scaledefinition.NewStage(value))
	}
	return out
}

func applicableAgesFromStrings(values []string) []scaledefinition.ApplicableAge {
	out := make([]scaledefinition.ApplicableAge, 0, len(values))
	for _, value := range values {
		out = append(out, scaledefinition.NewApplicableAge(value))
	}
	return out
}

func reportersFromStrings(values []string) []scaledefinition.Reporter {
	out := make([]scaledefinition.Reporter, 0, len(values))
	for _, value := range values {
		out = append(out, scaledefinition.NewReporter(value))
	}
	return out
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
		currentVersion = scaledefinition.DefaultScaleVersion
	}
	snapshot.ScaleVersion = scaledefinition.NewScaleVersion(currentVersion).IncrementPatch().String()
	snapshot.Status = scaledefinition.StatusDraft.String()
	if err := applyScaleSnapshotEnvelope(model, snapshot); err != nil {
		return err
	}
	return model.ForkDraftFromPublished(now)
}

// SyncScaleMetadataInModel projects editable metadata onto the legacy scale payload envelope.
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
		ScaleVersion:         scaledefinition.DefaultScaleVersion,
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

// MedicalScaleFromAssessmentModel materializes a legacy medical-scale aggregate from
// the target AssessmentModel for transitional factor authoring mutations.
func MedicalScaleFromAssessmentModel(model *domain.AssessmentModel) (*scaledefinition.MedicalScale, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	snapshot, err := scaleSnapshotEnvelope(model)
	if err != nil {
		return nil, err
	}
	factors, err := medicalScaleFactorsFromSnapshot(snapshot.Factors)
	if err != nil {
		return nil, err
	}
	status := scaleStatusFromAssessmentModel(model, snapshot.Status)
	opts := []scaledefinition.MedicalScaleOption{
		scaledefinition.WithDescription(model.Description),
		scaledefinition.WithCategory(scaledefinition.NewCategory(model.Category)),
		scaledefinition.WithStages(stagesFromStrings(model.Stages)),
		scaledefinition.WithApplicableAges(applicableAgesFromStrings(model.ApplicableAges)),
		scaledefinition.WithReporters(reportersFromStrings(model.Reporters)),
		scaledefinition.WithTags(tagsFromStrings(model.Tags)),
		scaledefinition.WithScaleVersion(snapshot.ScaleVersion),
		scaledefinition.WithQuestionnaire(meta.NewCode(bindingQuestionnaireCode(model, snapshot)), snapshot.QuestionnaireVersion),
		scaledefinition.WithStatus(status),
		scaledefinition.WithFactors(factors),
		scaledefinition.WithCreatedAt(model.CreatedAt),
		scaledefinition.WithUpdatedAt(model.UpdatedAt),
	}
	return scaledefinition.NewMedicalScale(meta.NewCode(model.Code), model.Title, opts...)
}

// SyncAssessmentModelFromMedicalScale projects a legacy medical-scale mutation back
// onto the target AssessmentModel while preserving aggregate identity fields.
func SyncAssessmentModelFromMedicalScale(model *domain.AssessmentModel, scale *scaledefinition.MedicalScale, now time.Time) error {
	if model == nil {
		return fmt.Errorf("assessment model is nil")
	}
	if scale == nil {
		return fmt.Errorf("medical scale is nil")
	}
	synced, err := AssessmentModelFromMedicalScale(scale, now)
	if err != nil {
		return err
	}
	model.Title = synced.Title
	model.Description = synced.Description
	model.Category = synced.Category
	model.Stages = append([]string(nil), synced.Stages...)
	model.ApplicableAges = append([]string(nil), synced.ApplicableAges...)
	model.Reporters = append([]string(nil), synced.Reporters...)
	model.Tags = append([]string(nil), synced.Tags...)
	model.Binding = synced.Binding
	model.Status = synced.Status
	model.PublishedAt = synced.PublishedAt
	model.ArchivedAt = synced.ArchivedAt
	return model.UpdateDefinitionWithV2(synced.Definition, synced.DefinitionV2, now)
}

func bindingQuestionnaireCode(model *domain.AssessmentModel, snapshot *scalesnapshot.ScaleSnapshot) string {
	if model != nil && model.Binding.QuestionnaireCode != "" {
		return model.Binding.QuestionnaireCode
	}
	if snapshot != nil {
		return snapshot.QuestionnaireCode
	}
	return ""
}

func scaleStatusFromAssessmentModel(model *domain.AssessmentModel, snapshotStatus string) scaledefinition.Status {
	if model != nil {
		switch model.Status {
		case domain.ModelStatusPublished:
			return scaledefinition.StatusPublished
		case domain.ModelStatusArchived:
			return scaledefinition.StatusArchived
		case domain.ModelStatusDraft:
			return scaledefinition.StatusDraft
		}
	}
	if status, ok := scaledefinition.ParseStatus(snapshotStatus); ok {
		return status
	}
	return scaledefinition.StatusDraft
}

func medicalScaleFactorsFromSnapshot(factors []scalesnapshot.FactorSnapshot) ([]*scaledefinition.Factor, error) {
	if len(factors) == 0 {
		return nil, nil
	}
	out := make([]*scaledefinition.Factor, 0, len(factors))
	for _, item := range factors {
		factor, err := medicalScaleFactorFromSnapshot(item)
		if err != nil {
			return nil, err
		}
		out = append(out, factor)
	}
	return out, nil
}

func medicalScaleFactorFromSnapshot(snapshot scalesnapshot.FactorSnapshot) (*scaledefinition.Factor, error) {
	strategy := scaledefinition.ScoringStrategySum
	if snapshot.ScoringStrategy != "" {
		strategy = scaledefinition.ScoringStrategyCode(snapshot.ScoringStrategy)
	}
	questionCodes := make([]meta.Code, 0, len(snapshot.QuestionCodes))
	for _, code := range snapshot.QuestionCodes {
		questionCodes = append(questionCodes, meta.NewCode(code))
	}
	rules := make([]scaledefinition.InterpretationRule, 0, len(snapshot.InterpretRules))
	for _, rule := range snapshot.InterpretRules {
		rules = append(rules, scaledefinition.NewInterpretationRule(
			scaledefinition.NewScoreRange(rule.Min, rule.Max),
			scaledefinition.RiskLevel(rule.RiskLevel),
			rule.Conclusion,
			rule.Suggestion,
		))
	}
	params := scaledefinition.NewScoringParams()
	if len(snapshot.ScoringParams.CntOptionContents) > 0 {
		params = params.WithCntOptionContents(snapshot.ScoringParams.CntOptionContents)
	}
	return scaledefinition.NewFactor(
		scaledefinition.NewFactorCode(snapshot.Code),
		snapshot.Title,
		scaledefinition.WithIsTotalScore(snapshot.IsTotalScore),
		scaledefinition.WithQuestionCodes(questionCodes),
		scaledefinition.WithScoringStrategy(strategy),
		scaledefinition.WithScoringParams(params),
		scaledefinition.WithMaxScore(snapshot.MaxScore),
		scaledefinition.WithInterpretRules(rules),
	)
}
