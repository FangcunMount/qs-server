// Package generation owns the semantic idempotency and lifecycle of one
// optional AI explanation.
package generation

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	aiexplanationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusGenerating Status = "generating"
	StatusGenerated  Status = "generated"
	StatusFailed     Status = "failed"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusGenerating, StatusGenerated, StatusFailed:
		return true
	default:
		return false
	}
}

// Key identifies one semantic generation. Requesting actor and trace identity
// deliberately do not participate in the key.
type Key struct {
	SourceReportID           meta.ID
	Audience                 policy.Audience
	Profile                  aiexplanation.ProfileRef
	InputFingerprint         aiexplanation.Fingerprint
	ExecutionSpecFingerprint aiexplanation.Fingerprint
}

func (k Key) Validate() error {
	if k.SourceReportID.IsZero() {
		return fmt.Errorf("AI explanation source report id is required")
	}
	if err := aiexplanation.ValidateAudience(k.Audience); err != nil {
		return err
	}
	if err := k.Profile.Validate(); err != nil {
		return err
	}
	if err := k.InputFingerprint.Validate(); err != nil {
		return err
	}
	return k.ExecutionSpecFingerprint.Validate()
}

// AIExplanationGeneration owns one frozen request, the latest attempt and the
// immutable successful Artifact reference. It never mutates the source report.
type AIExplanationGeneration struct {
	id            meta.ID
	key           Key
	association   aiexplanation.Association
	requestedBy   aiexplanation.ActorRef
	input         aiexplanationinput.Snapshot
	prompt        aiexplanation.PromptRef
	executionSpec aiexplanation.ProviderExecutionSpec
	status        Status
	latestRunID   meta.ID
	artifactID    meta.ID
	version       uint64
	createdAt     time.Time
	updatedAt     time.Time
}

type NewInput struct {
	ID            meta.ID
	Key           Key
	Association   aiexplanation.Association
	RequestedBy   aiexplanation.ActorRef
	Input         aiexplanationinput.Snapshot
	Prompt        aiexplanation.PromptRef
	ExecutionSpec aiexplanation.ProviderExecutionSpec
	CreatedAt     time.Time
}

func New(input NewInput) (*AIExplanationGeneration, error) {
	if input.ID.IsZero() {
		return nil, fmt.Errorf("AI explanation generation id is required")
	}
	if err := validateFrozenRequest(input.Key, input.Association, input.RequestedBy, input.Input, input.Prompt, input.ExecutionSpec); err != nil {
		return nil, err
	}
	if input.CreatedAt.IsZero() {
		return nil, fmt.Errorf("AI explanation generation created at is required")
	}
	return &AIExplanationGeneration{
		id: input.ID, key: input.Key, association: input.Association, requestedBy: input.RequestedBy,
		input: input.Input, prompt: input.Prompt, executionSpec: input.ExecutionSpec,
		status: StatusPending, version: 1, createdAt: input.CreatedAt, updatedAt: input.CreatedAt,
	}, nil
}

type RestoreInput struct {
	NewInput
	Status      Status
	LatestRunID meta.ID
	ArtifactID  meta.ID
	Version     uint64
	UpdatedAt   time.Time
}

func Restore(input RestoreInput) (*AIExplanationGeneration, error) {
	generation, err := New(input.NewInput)
	if err != nil {
		return nil, err
	}
	if !input.Status.IsValid() || input.Version == 0 || input.UpdatedAt.IsZero() || input.UpdatedAt.Before(input.CreatedAt) {
		return nil, fmt.Errorf("AI explanation generation persistence state is invalid")
	}
	switch input.Status {
	case StatusPending:
		if !input.LatestRunID.IsZero() || !input.ArtifactID.IsZero() {
			return nil, fmt.Errorf("pending AI explanation generation has execution references")
		}
	case StatusGenerating, StatusFailed:
		if input.LatestRunID.IsZero() || !input.ArtifactID.IsZero() {
			return nil, fmt.Errorf("AI explanation generation execution references are invalid")
		}
	case StatusGenerated:
		if input.LatestRunID.IsZero() || input.ArtifactID.IsZero() {
			return nil, fmt.Errorf("generated AI explanation references are required")
		}
	}
	generation.status = input.Status
	generation.latestRunID = input.LatestRunID
	generation.artifactID = input.ArtifactID
	generation.version = input.Version
	generation.updatedAt = input.UpdatedAt
	return generation, nil
}

func validateFrozenRequest(
	key Key,
	association aiexplanation.Association,
	requestedBy aiexplanation.ActorRef,
	snapshot aiexplanationinput.Snapshot,
	prompt aiexplanation.PromptRef,
	executionSpec aiexplanation.ProviderExecutionSpec,
) error {
	if err := key.Validate(); err != nil {
		return err
	}
	if err := association.Validate(); err != nil {
		return err
	}
	if err := requestedBy.Validate(); err != nil {
		return err
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if err := prompt.Validate(); err != nil {
		return err
	}
	if err := executionSpec.Validate(); err != nil {
		return err
	}
	if snapshot.Fingerprint() != key.InputFingerprint {
		return fmt.Errorf("AI explanation input snapshot does not match generation key")
	}
	if executionSpec.Fingerprint != key.ExecutionSpecFingerprint {
		return fmt.Errorf("AI explanation provider execution spec does not match generation key")
	}
	return nil
}

func (g *AIExplanationGeneration) Begin(runID meta.ID, at time.Time) error {
	if g == nil || runID.IsZero() || at.IsZero() {
		return fmt.Errorf("AI explanation generation, run id and start time are required")
	}
	if g.status != StatusPending && g.status != StatusFailed {
		return fmt.Errorf("AI explanation generation cannot begin from status %s", g.status)
	}
	g.status = StatusGenerating
	g.latestRunID = runID
	g.updatedAt = at
	g.version++
	return nil
}

func (g *AIExplanationGeneration) Succeed(runID, artifactID meta.ID, at time.Time) error {
	if g == nil || runID.IsZero() || artifactID.IsZero() || at.IsZero() {
		return fmt.Errorf("AI explanation generation success references are required")
	}
	if g.status != StatusGenerating || g.latestRunID != runID {
		return fmt.Errorf("AI explanation generation cannot succeed for run %s from status %s", runID, g.status)
	}
	g.status = StatusGenerated
	g.artifactID = artifactID
	g.updatedAt = at
	g.version++
	return nil
}

func (g *AIExplanationGeneration) Fail(runID meta.ID, at time.Time) error {
	if g == nil || runID.IsZero() || at.IsZero() {
		return fmt.Errorf("AI explanation generation failure references are required")
	}
	if g.status != StatusGenerating || g.latestRunID != runID {
		return fmt.Errorf("AI explanation generation cannot fail for run %s from status %s", runID, g.status)
	}
	g.status = StatusFailed
	g.updatedAt = at
	g.version++
	return nil
}

func (g *AIExplanationGeneration) ID() meta.ID                            { return g.id }
func (g *AIExplanationGeneration) Key() Key                               { return g.key }
func (g *AIExplanationGeneration) Association() aiexplanation.Association { return g.association }
func (g *AIExplanationGeneration) RequestedBy() aiexplanation.ActorRef    { return g.requestedBy }
func (g *AIExplanationGeneration) Input() aiexplanationinput.Snapshot     { return g.input }
func (g *AIExplanationGeneration) Prompt() aiexplanation.PromptRef        { return g.prompt }
func (g *AIExplanationGeneration) ExecutionSpec() aiexplanation.ProviderExecutionSpec {
	return g.executionSpec
}
func (g *AIExplanationGeneration) Status() Status       { return g.status }
func (g *AIExplanationGeneration) LatestRunID() meta.ID { return g.latestRunID }
func (g *AIExplanationGeneration) ArtifactID() meta.ID  { return g.artifactID }
func (g *AIExplanationGeneration) Version() uint64      { return g.version }
func (g *AIExplanationGeneration) CreatedAt() time.Time { return g.createdAt }
func (g *AIExplanationGeneration) UpdatedAt() time.Time { return g.updatedAt }
