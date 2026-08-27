// Package artifact owns a successful immutable AI explanation. Lifecycle and
// failure state remain in Generation and Run.
package artifact

import (
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type SourceRef struct {
	ReportID             meta.ID
	OutcomeID            meta.ID
	Association          aiexplanation.Association
	ReportType           string
	TemplateVersion      string
	ContentSchemaVersion string
	BuilderIdentity      string
	ReportGeneratedAt    time.Time
}

func (s SourceRef) Validate() error {
	if s.ReportID.IsZero() || s.OutcomeID.IsZero() || s.ReportType != "standard" {
		return fmt.Errorf("AI explanation standard source report and outcome are required")
	}
	if err := s.Association.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(s.TemplateVersion) == "" || strings.TrimSpace(s.ContentSchemaVersion) == "" || strings.TrimSpace(s.BuilderIdentity) == "" || s.ReportGeneratedAt.IsZero() {
		return fmt.Errorf("AI explanation source report provenance is required")
	}
	return nil
}

type ValidationReceipt struct {
	SchemaValidatorVersion    string
	ReferenceValidatorVersion string
	ProfileValidatorVersion   string
	SafetyValidatorVersion    string
	ValidatedAt               time.Time
}

func (r ValidationReceipt) Validate() error {
	if strings.TrimSpace(r.SchemaValidatorVersion) == "" || strings.TrimSpace(r.ReferenceValidatorVersion) == "" || strings.TrimSpace(r.ProfileValidatorVersion) == "" || strings.TrimSpace(r.SafetyValidatorVersion) == "" || r.ValidatedAt.IsZero() {
		return fmt.Errorf("AI explanation validation receipt is incomplete")
	}
	return nil
}

type AIExplanationArtifact struct {
	id               meta.ID
	generationID     meta.ID
	runID            meta.ID
	source           SourceRef
	audience         policy.Audience
	profile          aiexplanation.ProfileRef
	prompt           aiexplanation.PromptRef
	executionSpec    aiexplanation.ProviderExecutionSpec
	inputSchema      string
	inputFingerprint aiexplanation.Fingerprint
	outputSchema     string
	safetyPolicy     string
	providerReceipt  aiexplanation.ProviderReceipt
	validation       ValidationReceipt
	content          output.Content
	generatedAt      time.Time
}

type NewInput struct {
	ID               meta.ID
	GenerationID     meta.ID
	RunID            meta.ID
	Source           SourceRef
	Audience         policy.Audience
	Profile          aiexplanation.ProfileRef
	Prompt           aiexplanation.PromptRef
	ExecutionSpec    aiexplanation.ProviderExecutionSpec
	InputSchema      string
	InputFingerprint aiexplanation.Fingerprint
	OutputSchema     string
	SafetyPolicy     string
	ProviderReceipt  aiexplanation.ProviderReceipt
	Validation       ValidationReceipt
	Content          output.Content
	GeneratedAt      time.Time
}

func New(input NewInput) (*AIExplanationArtifact, error) {
	if input.ID.IsZero() || input.GenerationID.IsZero() || input.RunID.IsZero() {
		return nil, fmt.Errorf("AI explanation artifact, generation and run ids are required")
	}
	if err := input.Source.Validate(); err != nil {
		return nil, err
	}
	if err := aiexplanation.ValidateAudience(input.Audience); err != nil {
		return nil, err
	}
	if err := input.Profile.Validate(); err != nil {
		return nil, err
	}
	if err := input.Prompt.Validate(); err != nil {
		return nil, err
	}
	if err := input.ExecutionSpec.Validate(); err != nil {
		return nil, err
	}
	if input.InputSchema != aiexplanation.InputSchemaVersionV1 || input.OutputSchema != aiexplanation.OutputSchemaVersionV1 {
		return nil, fmt.Errorf("AI explanation artifact schema versions are invalid")
	}
	if err := input.InputFingerprint.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.SafetyPolicy) == "" {
		return nil, fmt.Errorf("AI explanation safety policy version is required")
	}
	if err := input.ProviderReceipt.Validate(); err != nil {
		return nil, err
	}
	if input.ProviderReceipt.Provider != input.ExecutionSpec.ResolvedProvider || input.ProviderReceipt.Model != input.ExecutionSpec.ResolvedModel {
		return nil, fmt.Errorf("AI explanation provider receipt does not match execution spec")
	}
	if err := input.Validation.Validate(); err != nil {
		return nil, err
	}
	if err := input.Content.Validate(); err != nil {
		return nil, err
	}
	if input.Content.SchemaVersion != input.OutputSchema {
		return nil, fmt.Errorf("AI explanation output content schema mismatch")
	}
	if input.GeneratedAt.IsZero() || input.GeneratedAt.Before(input.Validation.ValidatedAt) {
		return nil, fmt.Errorf("AI explanation artifact generation time is invalid")
	}
	return &AIExplanationArtifact{
		id: input.ID, generationID: input.GenerationID, runID: input.RunID,
		source: input.Source, audience: input.Audience, profile: input.Profile, prompt: input.Prompt,
		executionSpec: input.ExecutionSpec, inputSchema: input.InputSchema, inputFingerprint: input.InputFingerprint,
		outputSchema: input.OutputSchema, safetyPolicy: input.SafetyPolicy, providerReceipt: input.ProviderReceipt,
		validation: input.Validation, content: input.Content.Clone(), generatedAt: input.GeneratedAt,
	}, nil
}

func (a *AIExplanationArtifact) ID() meta.ID                       { return a.id }
func (a *AIExplanationArtifact) GenerationID() meta.ID             { return a.generationID }
func (a *AIExplanationArtifact) RunID() meta.ID                    { return a.runID }
func (a *AIExplanationArtifact) Source() SourceRef                 { return a.source }
func (a *AIExplanationArtifact) Audience() policy.Audience         { return a.audience }
func (a *AIExplanationArtifact) Profile() aiexplanation.ProfileRef { return a.profile }
func (a *AIExplanationArtifact) Prompt() aiexplanation.PromptRef   { return a.prompt }
func (a *AIExplanationArtifact) ExecutionSpec() aiexplanation.ProviderExecutionSpec {
	return a.executionSpec
}
func (a *AIExplanationArtifact) InputSchema() string { return a.inputSchema }
func (a *AIExplanationArtifact) InputFingerprint() aiexplanation.Fingerprint {
	return a.inputFingerprint
}
func (a *AIExplanationArtifact) OutputSchema() string { return a.outputSchema }
func (a *AIExplanationArtifact) SafetyPolicy() string { return a.safetyPolicy }
func (a *AIExplanationArtifact) ProviderReceipt() aiexplanation.ProviderReceipt {
	return a.providerReceipt
}
func (a *AIExplanationArtifact) Validation() ValidationReceipt { return a.validation }
func (a *AIExplanationArtifact) Content() output.Content       { return a.content.Clone() }
func (a *AIExplanationArtifact) GeneratedAt() time.Time        { return a.generatedAt }
