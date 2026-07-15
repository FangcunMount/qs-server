package report

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// InterpretReport is the immutable successful report produced by one
// InterpretationRun under a ReportGeneration.
type InterpretReport struct {
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

type InterpretReportInput struct {
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

func NewInterpretReport(input InterpretReportInput) (*InterpretReport, error) {
	if input.ID.IsZero() || input.GenerationID.IsZero() || input.OutcomeID.IsZero() || input.InterpretationRunID.IsZero() {
		return nil, fmt.Errorf("report, generation, outcome and interpretation run ids are required")
	}
	if input.Association.OrgID == 0 || input.Association.AssessmentID.IsZero() || input.Association.TesteeID == 0 {
		return nil, fmt.Errorf("report organization, assessment and testee association are required")
	}
	if input.ReportType.IsEmpty() || input.TemplateVersion.IsEmpty() {
		return nil, fmt.Errorf("report type and template version are required")
	}
	if input.GeneratedAt.IsZero() {
		return nil, fmt.Errorf("report generated at is required")
	}
	return &InterpretReport{
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

func (r *InterpretReport) ID() meta.ID { return r.id }

func (r *InterpretReport) GenerationID() meta.ID { return r.generationID }

func (r *InterpretReport) OutcomeID() meta.ID { return r.outcomeID }

func (r *InterpretReport) InterpretationRunID() meta.ID { return r.interpretationRunID }

func (r *InterpretReport) Association() Association { return r.association }

func (r *InterpretReport) ReportType() policy.ReportType { return r.reportType }

func (r *InterpretReport) TemplateVersion() policy.TemplateVersion { return r.templateVersion }

func (r *InterpretReport) Content() Content { return cloneContent(r.content) }

func (r *InterpretReport) GeneratedAt() time.Time { return r.generatedAt }

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
		cloned[i].derivedScores = cloneScoreValues(item.derivedScores)
		cloned[i].level = cloneResultLevel(item.level)
		cloned[i].normReference = cloneNormReference(item.normReference)
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
