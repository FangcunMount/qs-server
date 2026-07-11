package report

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Artifact is the target immutable InterpretReport. It is introduced beside
// the legacy lifecycle-bearing InterpretReport in Batch I1; Batch I2/I8 move
// persistence and callers to this type before the legacy aggregate is removed.
type Artifact struct {
	id                  meta.ID
	generationID        meta.ID
	outcomeID           meta.ID
	interpretationRunID meta.ID
	association         Association
	reportType          policy.ReportType
	templateVersion     policy.TemplateVersion
	content             Content
	generatedAt         time.Time
}

// Content is the immutable report payload. It intentionally has no lifecycle,
// attempt or failure state.
type Content struct {
	Model        ModelIdentity
	PrimaryScore *ScoreValue
	Level        *ResultLevel
	Conclusion   string
	Dimensions   []DimensionInterpret
	Suggestions  []Suggestion
	ModelExtra   *ModelExtra
}

// Association is a frozen read-side correlation copied from EvaluationOutcome.
// It is not an Assessment aggregate reference and grants no write authority.
type Association struct {
	OrgID        int64
	AssessmentID meta.ID
	TesteeID     uint64
}

type ArtifactInput struct {
	ID                  meta.ID
	GenerationID        meta.ID
	OutcomeID           meta.ID
	InterpretationRunID meta.ID
	Association         Association
	ReportType          policy.ReportType
	TemplateVersion     policy.TemplateVersion
	Content             Content
	GeneratedAt         time.Time
}

func NewArtifact(input ArtifactInput) (*Artifact, error) {
	if input.ID.IsZero() || input.GenerationID.IsZero() || input.OutcomeID.IsZero() || input.InterpretationRunID.IsZero() {
		return nil, fmt.Errorf("report, generation, outcome and interpretation run ids are required")
	}
	if input.Association.AssessmentID.IsZero() || input.Association.TesteeID == 0 {
		return nil, fmt.Errorf("report assessment and testee association are required")
	}
	if input.ReportType.IsEmpty() || input.TemplateVersion.IsEmpty() {
		return nil, fmt.Errorf("report type and template version are required")
	}
	if input.GeneratedAt.IsZero() {
		return nil, fmt.Errorf("report generated at is required")
	}
	return &Artifact{
		id:                  input.ID,
		generationID:        input.GenerationID,
		outcomeID:           input.OutcomeID,
		interpretationRunID: input.InterpretationRunID,
		association:         input.Association,
		reportType:          input.ReportType,
		templateVersion:     input.TemplateVersion,
		content:             cloneContent(input.Content),
		generatedAt:         input.GeneratedAt,
	}, nil
}

func (a *Artifact) ID() meta.ID { return a.id }

func (a *Artifact) GenerationID() meta.ID { return a.generationID }

func (a *Artifact) OutcomeID() meta.ID { return a.outcomeID }

func (a *Artifact) InterpretationRunID() meta.ID { return a.interpretationRunID }

func (a *Artifact) Association() Association { return a.association }

func (a *Artifact) ReportType() policy.ReportType { return a.reportType }

func (a *Artifact) TemplateVersion() policy.TemplateVersion { return a.templateVersion }

func (a *Artifact) Content() Content { return cloneContent(a.content) }

func (a *Artifact) GeneratedAt() time.Time { return a.generatedAt }

func cloneContent(content Content) Content {
	cloned := Content{
		Model:       content.Model,
		Conclusion:  content.Conclusion,
		Dimensions:  cloneDimensions(content.Dimensions),
		Suggestions: cloneSuggestions(content.Suggestions),
	}
	if content.PrimaryScore != nil {
		cloned.PrimaryScore = &ScoreValue{Kind: content.PrimaryScore.Kind, Value: content.PrimaryScore.Value, Label: content.PrimaryScore.Label}
		if content.PrimaryScore.Max != nil {
			max := *content.PrimaryScore.Max
			cloned.PrimaryScore.Max = &max
		}
	}
	if content.Level != nil {
		cloned.Level = &ResultLevel{Code: content.Level.Code, Label: content.Level.Label, Severity: content.Level.Severity}
	}
	if content.ModelExtra != nil {
		cloned.ModelExtra = &ModelExtra{
			Kind:           content.ModelExtra.Kind,
			TypeCode:       content.ModelExtra.TypeCode,
			TypeName:       content.ModelExtra.TypeName,
			OneLiner:       content.ModelExtra.OneLiner,
			ImageURL:       content.ModelExtra.ImageURL,
			MatchPercent:   content.ModelExtra.MatchPercent,
			IsSpecial:      content.ModelExtra.IsSpecial,
			SpecialTrigger: content.ModelExtra.SpecialTrigger,
			Commentary:     content.ModelExtra.Commentary,
		}
		if content.ModelExtra.Rarity != nil {
			cloned.ModelExtra.Rarity = &ModelRarity{Percent: content.ModelExtra.Rarity.Percent, Label: content.ModelExtra.Rarity.Label, OneInX: content.ModelExtra.Rarity.OneInX}
		}
	}
	return cloned
}

func cloneDimensions(items []DimensionInterpret) []DimensionInterpret {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]DimensionInterpret, len(items))
	for i, item := range items {
		cloned[i] = item
		if item.maxScore != nil {
			max := *item.maxScore
			cloned[i].maxScore = &max
		}
	}
	return cloned
}

func cloneSuggestions(items []Suggestion) []Suggestion {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]Suggestion, len(items))
	for i, item := range items {
		cloned[i] = item
		if item.FactorCode != nil {
			code := *item.FactorCode
			cloned[i].FactorCode = &code
		}
	}
	return cloned
}
