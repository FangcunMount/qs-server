package interpretation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
		BaseDocument: base.BaseDocument{DomainID: domain.ID()},
		GenerationID: domain.GenerationID().Uint64(),
		Attempt:      domain.Attempt(),
		Status:       string(domain.Status()),
		TraceID:      domain.TraceID(),
		StartedAt:    domain.StartedAt(),
		FinishedAt:   domain.FinishedAt(),
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
	return interpretationrun.Restore(interpretationrun.RestoreInput{
		ID:           po.DomainID,
		GenerationID: meta.FromUint64(po.GenerationID),
		Attempt:      po.Attempt,
		Status:       interpretationrun.Status(po.Status),
		Failure:      failure,
		TraceID:      po.TraceID,
		StartedAt:    po.StartedAt,
		FinishedAt:   po.FinishedAt,
	})
}

func (m *LifecycleMapper) ArtifactToPO(domain *domainreport.Artifact) *InterpretReportArtifactPO {
	if domain == nil {
		return nil
	}
	content := domain.Content()
	association := domain.Association()
	po := &InterpretReportArtifactPO{
		BaseDocument:        base.BaseDocument{DomainID: domain.ID(), CreatedAt: domain.GeneratedAt(), UpdatedAt: domain.GeneratedAt()},
		GenerationID:        domain.GenerationID().Uint64(),
		OutcomeID:           domain.OutcomeID().Uint64(),
		InterpretationRunID: domain.InterpretationRunID().Uint64(),
		ReportType:          domain.ReportType().String(),
		TemplateVersion:     domain.TemplateVersion().String(),
		GeneratedAt:         domain.GeneratedAt(),
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

func (m *LifecycleMapper) ArtifactToDomain(po *InterpretReportArtifactPO) (*domainreport.Artifact, error) {
	if po == nil {
		return nil, nil
	}
	artifact, err := domainreport.NewArtifact(domainreport.ArtifactInput{
		ID:                  po.DomainID,
		GenerationID:        meta.FromUint64(po.GenerationID),
		OutcomeID:           meta.FromUint64(po.OutcomeID),
		InterpretationRunID: meta.FromUint64(po.InterpretationRunID),
		Association:         domainreport.Association{OrgID: po.OrgID, AssessmentID: meta.FromUint64(po.AssessmentID), TesteeID: po.TesteeID},
		ReportType:          policy.ReportType(po.ReportType),
		TemplateVersion:     policy.TemplateVersion(po.TemplateVersion),
		Content: domainreport.Content{
			Model:        modelIdentityToDomain(po.Model),
			PrimaryScore: scoreValueToDomain(po.PrimaryScore),
			Level:        resultLevelToDomain(po.Level),
			Conclusion:   po.Conclusion,
			Dimensions:   dimensionsToDomain(po.Dimensions),
			Suggestions:  toDomainSuggestions(po.Suggestions),
			ModelExtra:   toDomainModelExtra(po.ModelExtra),
		},
		GeneratedAt: po.GeneratedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("restore interpretation artifact: %w", err)
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
