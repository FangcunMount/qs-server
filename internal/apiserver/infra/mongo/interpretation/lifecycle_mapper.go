package interpretation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type LifecycleMapper struct{}

func NewLifecycleMapper() *LifecycleMapper { return &LifecycleMapper{} }

func (m *LifecycleMapper) GenerationToPO(domain *generation.ReportGeneration) *ReportGenerationPO {
	if domain == nil {
		return nil
	}
	return &ReportGenerationPO{
		BaseDocument:    base.BaseDocument{DomainID: domain.ID(), CreatedAt: domain.CreatedAt(), UpdatedAt: domain.UpdatedAt()},
		OutcomeID:       domain.Key().OutcomeID.Uint64(),
		ReportType:      domain.Key().ReportType.String(),
		TemplateVersion: domain.Key().TemplateVersion.String(),
		Status:          string(domain.Status()),
		LatestRunID:     domain.LatestRunID().Uint64(),
		ReportID:        domain.ReportID().Uint64(),
		Version:         domain.Version(),
	}
}

func (m *LifecycleMapper) GenerationToDomain(po *ReportGenerationPO) (*generation.ReportGeneration, error) {
	if po == nil {
		return nil, nil
	}
	return generation.Restore(generation.RestoreInput{
		ID:          po.DomainID,
		Key:         generation.Key{OutcomeID: meta.FromUint64(po.OutcomeID), ReportType: policy.ReportType(po.ReportType), TemplateVersion: policy.TemplateVersion(po.TemplateVersion)},
		Status:      generation.Status(po.Status),
		LatestRunID: meta.FromUint64(po.LatestRunID),
		ReportID:    meta.FromUint64(po.ReportID),
		Version:     po.Version,
		CreatedAt:   po.CreatedAt,
		UpdatedAt:   po.UpdatedAt,
	})
}

func (m *LifecycleMapper) RunToPO(domain *interpretationrun.InterpretationRun) *InterpretationRunPO {
	if domain == nil {
		return nil
	}
	po := &InterpretationRunPO{
		BaseDocument:   base.BaseDocument{DomainID: domain.ID()},
		GenerationID:   domain.GenerationID().Uint64(),
		Attempt:        domain.Attempt(),
		Status:         string(domain.Status()),
		TraceID:        domain.TraceID(),
		StartedAt:      domain.StartedAt(),
		LeaseExpiresAt: domain.LeaseExpiresAt(),
		FinishedAt:     domain.FinishedAt(),
		AttemptOrigin:  string(domain.Origin()),
		RecoveryCount:  domain.RecoveryCount(),
		LastReclaimedAt: domain.LastReclaimedAt(),
	}
	if history := domain.ClaimHistory(); len(history) > 0 {
		po.ClaimHistory = make([]ClaimHistoryPO, len(history))
		for i, record := range history {
			po.ClaimHistory[i] = ClaimHistoryPO{ReclaimedAt: record.ReclaimedAt, TraceID: record.TraceID}
		}
	}
	if decision := domain.RetryDecision(); decision != nil {
		po.RetryDisposition = string(decision.Disposition)
		po.NextAttemptAt = decision.NextAttemptAt
		po.PolicyMaxAttempts = decision.MaxAutomaticAttempts
		po.RetryPolicyVersion = decision.PolicyVersion
		po.RetryEventID = decision.RetryEventID
		po.ActionRequestID = decision.ActionRequestID
	}
	if failure := domain.Failure(); failure != nil {
		po.Failure = &InterpretationFailurePO{Kind: string(failure.Kind), Code: failure.Code, SafeMessage: failure.SafeMessage, Retryable: failure.Retryable}
	}
	return po
}

func (m *LifecycleMapper) RunToDomain(po *InterpretationRunPO) (*interpretationrun.InterpretationRun, error) {
	if po == nil {
		return nil, nil
	}
	var failure *interpretationrun.Failure
	if po.Failure != nil {
		failure = &interpretationrun.Failure{Kind: interpretationrun.FailureKind(po.Failure.Kind), Code: po.Failure.Code, SafeMessage: po.Failure.SafeMessage, Retryable: po.Failure.Retryable}
	}
	restore := interpretationrun.RestoreInput{
		ID:             po.DomainID,
		GenerationID:   meta.FromUint64(po.GenerationID),
		Attempt:        po.Attempt,
		Status:         interpretationrun.Status(po.Status),
		Failure:        failure,
		TraceID:        po.TraceID,
		StartedAt:      po.StartedAt,
		LeaseExpiresAt: po.LeaseExpiresAt,
		FinishedAt:     po.FinishedAt,
		Origin:          retrygovernance.AttemptOrigin(po.AttemptOrigin),
		RecoveryCount:   po.RecoveryCount,
		LastReclaimedAt: po.LastReclaimedAt,
	}
	if len(po.ClaimHistory) > 0 {
		restore.ClaimHistory = make([]interpretationrun.ClaimRecord, len(po.ClaimHistory))
		for i, record := range po.ClaimHistory {
			restore.ClaimHistory[i] = interpretationrun.ClaimRecord{ReclaimedAt: record.ReclaimedAt, TraceID: record.TraceID}
		}
	}
	if po.RetryDisposition != "" {
		restore.RetryDecision = &retrygovernance.Decision{
			Disposition: retrygovernance.Disposition(po.RetryDisposition), Attempt: po.Attempt,
			MaxAutomaticAttempts:       po.PolicyMaxAttempts,
			RemainingAutomaticAttempts: max(po.PolicyMaxAttempts-po.Attempt, 0),
			NextAttemptAt:              po.NextAttemptAt, PolicyVersion: po.RetryPolicyVersion,
			RetryEventID: po.RetryEventID, ActionRequestID: po.ActionRequestID,
		}
	} else if failure != nil {
		decisionAt := po.UpdatedAt
		if po.FinishedAt != nil {
			decisionAt = *po.FinishedAt
		}
		decision := retrygovernance.BusinessPolicy().DecideFailure(failure.Retryable, po.Attempt, decisionAt)
		restore.RetryDecision = &decision
	}
	return interpretationrun.Restore(restore)
}

func (m *LifecycleMapper) ReportToPO(domain *domainreport.InterpretReport) *InterpretReportPO {
	if domain == nil {
		return nil
	}
	content := domain.Content()
	association := domain.Association()
	po := &InterpretReportPO{
		BaseDocument:        base.BaseDocument{DomainID: domain.ID(), CreatedAt: domain.GeneratedAt(), UpdatedAt: domain.GeneratedAt()},
		GenerationID:        domain.GenerationID().Uint64(),
		OutcomeID:           domain.OutcomeID().Uint64(),
		InterpretationRunID: domain.InterpretationRunID().Uint64(),
		ReportType:           domain.ReportType().String(),
		TemplateVersion:      domain.TemplateVersion().String(),
		BuilderIdentity:      domain.BuilderIdentity(),
		ContentSchemaVersion: domain.ContentSchemaVersion(),
		GeneratedAt:          domain.GeneratedAt(),
		OrgID:               association.OrgID,
		AssessmentID:        association.AssessmentID.Uint64(),
		TesteeID:            association.TesteeID,
		ScaleName:           content.Model.Title,
		ScaleCode:           content.Model.Code,
		Model:               modelIdentityToPO(content.Model),
		PrimaryScore:        scoreValueToPO(content.PrimaryScore),
		Level:               resultLevelToPO(content.Level),
		Conclusion:          content.Conclusion,
		Dimensions:          dimensionsToPO(content.Dimensions),
		Suggestions:         toSuggestionPOs(content.Suggestions),
		ModelExtra:          toModelExtraPO(content.ModelExtra),
		PresentationProfile: presentationProfileToPO(content.PresentationProfile),
	}
	if content.PrimaryScore != nil {
		po.TotalScore = content.PrimaryScore.Value
	}
	if content.Level != nil && isArtifactRiskLevelCode(content.Level.Code) {
		po.RiskLevel = content.Level.Code
	}
	return po
}

func isArtifactRiskLevelCode(code string) bool {
	switch code {
	case "none", "low", "medium", "high", "severe":
		return true
	default:
		return false
	}
}

func (m *LifecycleMapper) ReportToDomain(po *InterpretReportPO) (*domainreport.InterpretReport, error) {
	if po == nil {
		return nil, nil
	}
	artifact, err := domainreport.RestoreInterpretReport(domainreport.InterpretReportInput{
		ID:                   po.DomainID,
		GenerationID:         meta.FromUint64(po.GenerationID),
		OutcomeID:            meta.FromUint64(po.OutcomeID),
		InterpretationRunID:  meta.FromUint64(po.InterpretationRunID),
		Association:          domainreport.Association{OrgID: po.OrgID, AssessmentID: meta.FromUint64(po.AssessmentID), TesteeID: po.TesteeID},
		ReportType:           policy.ReportType(po.ReportType),
		TemplateVersion:      policy.TemplateVersion(po.TemplateVersion),
		BuilderIdentity:      po.BuilderIdentity,
		ContentSchemaVersion: po.ContentSchemaVersion,
		Content: domainreport.Content{
			Model:        modelIdentityToDomain(po.Model),
			PrimaryScore: scoreValueToDomain(po.PrimaryScore),
			Level:        resultLevelToDomain(po.Level),
			Conclusion:   po.Conclusion,
			Dimensions:   dimensionsToDomain(po.Dimensions),
			Suggestions:  toDomainSuggestions(po.Suggestions),
			ModelExtra:   toDomainModelExtra(po.ModelExtra),
			PresentationProfile: presentationProfileToDomain(po.PresentationProfile),
		},
		GeneratedAt: po.GeneratedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("restore interpretation report: %w", err)
	}
	return artifact, nil
}

func dimensionsToPO(items []domainreport.DimensionInterpret) []DimensionInterpretPO {
	if len(items) == 0 {
		return nil
	}
	result := make([]DimensionInterpretPO, len(items))
	for i, item := range items {
		result[i] = dimensionToPO(item)
	}
	return result
}

func dimensionsToDomain(items []DimensionInterpretPO) []domainreport.DimensionInterpret {
	if len(items) == 0 {
		return nil
	}
	result := make([]domainreport.DimensionInterpret, len(items))
	for i, item := range items {
		result[i] = dimensionToDomain(item)
	}
	return result
}

func dimensionToPO(d domainreport.DimensionInterpret) DimensionInterpretPO {
	po := DimensionInterpretPO{
		Kind: string(d.Kind()), FactorCode: d.Code().String(), FactorName: d.Name(), RawScore: d.RawScore(), MaxScore: d.MaxScore(),
		RiskLevel: d.Severity(), Role: d.Role(), ParentCode: d.ParentCode(), HierarchyLevel: d.HierarchyLevel(), SortOrder: d.SortOrder(),
		Description: d.Description(), Suggestion: d.Suggestion(),
	}
	po.Score = scoreValueToPO(domainreport.NewRawTotalScore(d.RawScore(), d.MaxScore()))
	po.DerivedScores = scoreValuesToPO(d.DerivedScores())
	po.Level = resultLevelToPO(d.Level())
	po.NormReference = normReferenceToPO(d.NormReference())
	if po.Level == nil && d.Severity() != "none" && isArtifactRiskLevelCode(d.Severity()) {
		po.Level = resultLevelToPO(domainreport.LevelFromRisk(domainreport.RiskLevel(d.Severity())))
	}
	return po
}

func dimensionToDomain(po DimensionInterpretPO) domainreport.DimensionInterpret {
	rawScore, maxScore, risk := po.RawScore, po.MaxScore, domainreport.RiskLevel(po.RiskLevel)
	if score := scoreValueToDomain(po.Score); score != nil {
		rawScore, maxScore = score.Value, score.Max
	}
	if level := resultLevelToDomain(po.Level); level != nil && level.Code != "" {
		risk = domainreport.RiskLevel(level.Code)
	}
	kind := domainreport.DimensionKind(po.Kind)
	var dimension domainreport.DimensionInterpret
	if kind != "" && kind != domainreport.DimensionKindFactor {
		dimension = domainreport.NewNeutralDimensionInterpret(domainreport.NewDimensionCode(po.FactorCode), kind, po.FactorName, rawScore, maxScore, resultLevelToDomain(po.Level), po.Description, po.Suggestion)
	} else {
		dimension = domainreport.NewDimensionInterpret(domainreport.NewFactorCode(po.FactorCode), po.FactorName, rawScore, maxScore, risk, po.Description, po.Suggestion)
	}
	return dimension.WithScoreContext(scoreValuesToDomain(po.DerivedScores), resultLevelToDomain(po.Level), normReferenceToDomain(po.NormReference)).WithHierarchy(po.Role, po.ParentCode, po.HierarchyLevel, po.SortOrder)
}

func toSuggestionPOs(items []domainreport.Suggestion) []SuggestionPO {
	if len(items) == 0 {
		return nil
	}
	result := make([]SuggestionPO, len(items))
	for i, suggestion := range items {
		var factorCode *string
		if suggestion.FactorCode != nil {
			value := suggestion.FactorCode.String()
			factorCode = &value
		}
		result[i] = SuggestionPO{Category: string(suggestion.Category), Content: suggestion.Content, FactorCode: factorCode}
	}
	return result
}

func toDomainSuggestions(items []SuggestionPO) []domainreport.Suggestion {
	if len(items) == 0 {
		return nil
	}
	result := make([]domainreport.Suggestion, len(items))
	for i, suggestion := range items {
		var factorCode *domainreport.FactorCode
		if suggestion.FactorCode != nil {
			value := domainreport.NewFactorCode(*suggestion.FactorCode)
			factorCode = &value
		}
		result[i] = domainreport.Suggestion{Category: domainreport.SuggestionCategory(suggestion.Category), Content: suggestion.Content, FactorCode: factorCode}
	}
	return result
}

func toModelExtraPO(extra *domainreport.ModelExtra) *ModelExtraPO {
	if extra == nil || extra.IsEmpty() {
		return nil
	}
	po := &ModelExtraPO{Kind: extra.Kind, TypeCode: extra.TypeCode, TypeName: extra.TypeName, OneLiner: extra.OneLiner, ImageURL: extra.ImageURL, MatchPercent: extra.MatchPercent, IsSpecial: extra.IsSpecial, SpecialTrigger: extra.SpecialTrigger, Commentary: extra.Commentary}
	if extra.Rarity != nil {
		po.Rarity = &ModelRarityPO{Percent: extra.Rarity.Percent, Label: extra.Rarity.Label, OneInX: extra.Rarity.OneInX}
	}
	return po
}

func toDomainModelExtra(po *ModelExtraPO) *domainreport.ModelExtra {
	if po == nil {
		return nil
	}
	extra := &domainreport.ModelExtra{Kind: po.Kind, TypeCode: po.TypeCode, TypeName: po.TypeName, OneLiner: po.OneLiner, ImageURL: po.ImageURL, MatchPercent: po.MatchPercent, IsSpecial: po.IsSpecial, SpecialTrigger: po.SpecialTrigger, Commentary: po.Commentary}
	if po.Rarity != nil {
		extra.Rarity = &domainreport.ModelRarity{Percent: po.Rarity.Percent, Label: po.Rarity.Label, OneInX: po.Rarity.OneInX}
	}
	if extra.IsEmpty() {
		return nil
	}
	return extra
}

func modelIdentityToPO(model domainreport.ModelIdentity) *ModelIdentityPO {
	if model.IsEmpty() {
		return nil
	}
	return &ModelIdentityPO{Kind: model.Kind, SubKind: model.SubKind, Algorithm: model.Algorithm, Code: model.Code, Version: model.Version, Title: model.Title, ProductChannel: model.ProductChannel, AlgorithmFamily: model.AlgorithmFamily}
}

func modelIdentityToDomain(po *ModelIdentityPO) domainreport.ModelIdentity {
	if po == nil {
		return domainreport.ModelIdentity{}
	}
	return domainreport.ModelIdentity{Kind: po.Kind, SubKind: po.SubKind, Algorithm: po.Algorithm, Code: po.Code, Version: po.Version, Title: po.Title, ProductChannel: po.ProductChannel, AlgorithmFamily: po.AlgorithmFamily}
}

func scoreValueToPO(score *domainreport.ScoreValue) *ScoreValuePO {
	if score == nil {
		return nil
	}
	return &ScoreValuePO{Kind: score.Kind, Value: score.Value, Label: score.Label, Max: score.Max}
}

func scoreValueToDomain(po *ScoreValuePO) *domainreport.ScoreValue {
	if po == nil {
		return nil
	}
	return &domainreport.ScoreValue{Kind: po.Kind, Value: po.Value, Label: po.Label, Max: po.Max}
}

func resultLevelToPO(level *domainreport.ResultLevel) *ResultLevelPO {
	if level == nil {
		return nil
	}
	return &ResultLevelPO{Code: level.Code, Label: level.Label, Severity: level.Severity}
}

func resultLevelToDomain(po *ResultLevelPO) *domainreport.ResultLevel {
	if po == nil {
		return nil
	}
	return &domainreport.ResultLevel{Code: po.Code, Label: po.Label, Severity: po.Severity}
}

func scoreValuesToPO(scores []domainreport.ScoreValue) []ScoreValuePO {
	if len(scores) == 0 {
		return nil
	}
	result := make([]ScoreValuePO, len(scores))
	for i := range scores {
		result[i] = *scoreValueToPO(&scores[i])
	}
	return result
}

func scoreValuesToDomain(scores []ScoreValuePO) []domainreport.ScoreValue {
	if len(scores) == 0 {
		return nil
	}
	result := make([]domainreport.ScoreValue, len(scores))
	for i := range scores {
		result[i] = *scoreValueToDomain(&scores[i])
	}
	return result
}

func normReferenceToPO(reference *domainreport.NormReference) *NormReferencePO {
	if reference == nil {
		return nil
	}
	return &NormReferencePO{ScoreKind: reference.ScoreKind, Benchmark: reference.Benchmark, TableVersion: reference.TableVersion, FormVariant: reference.FormVariant, MinAgeMonths: reference.MinAgeMonths, MaxAgeMonths: reference.MaxAgeMonths, Gender: reference.Gender}
}

func normReferenceToDomain(reference *NormReferencePO) *domainreport.NormReference {
	if reference == nil {
		return nil
	}
	return &domainreport.NormReference{ScoreKind: reference.ScoreKind, Benchmark: reference.Benchmark, TableVersion: reference.TableVersion, FormVariant: reference.FormVariant, MinAgeMonths: reference.MinAgeMonths, MaxAgeMonths: reference.MaxAgeMonths, Gender: reference.Gender}
}

func presentationProfileToPO(profile *domainreport.PresentationProfile) *PresentationProfilePO {
	if profile == nil || profile.Source == "" {
		return nil
	}
	return &PresentationProfilePO{
		VisibleFactorCodes: append([]string(nil), profile.VisibleFactorCodes...),
		Source:             string(profile.Source),
	}
}

func presentationProfileToDomain(po *PresentationProfilePO) *domainreport.PresentationProfile {
	if po == nil || po.Source == "" {
		return nil
	}
	return &domainreport.PresentationProfile{
		VisibleFactorCodes: append([]string(nil), po.VisibleFactorCodes...),
		Source:             domainreport.PresentationProfileSource(po.Source),
	}
}
