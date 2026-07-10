package definition

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// SaveInput carries API-facing draft definition fields before family-specific
// normalization.
type SaveInput struct {
	PayloadFormat string
	Payload       []byte
	Algorithm     string
	SubKind       string
}

// SaveResult is the normalized draft definition update produced by a handler.
type SaveResult struct {
	Payload      domain.DefinitionPayload
	DefinitionV2 *domain.Definition
	Norms        []*norm.Norm
	Algorithm    domain.Algorithm
	SubKind      domain.SubKind
}

// SnapshotBuildResult carries only the family-specific pieces needed to
// materialize an AssessmentSnapshot.
type SnapshotBuildResult struct {
	Kind          domain.Kind
	SubKind       domain.SubKind
	Algorithm     domain.Algorithm
	PayloadFormat string
	DecisionKind  domain.DecisionKind
	Payload       []byte
	// Version optionally overrides the default draft revision version string.
	Version string
}

// Handler owns family-specific definition validation and publication shaping.
//
// Workbench use cases should route DefinitionPayload through this interface
// instead of switching on model family in each application service.
type Handler interface {
	Supports(identity domain.Identity) bool
	PrepareForSave(ctx context.Context, model *domain.AssessmentModel, input SaveInput) (SaveResult, []domain.DomainValidationIssue, error)
	ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue
	BuildSnapshotPayload(ctx context.Context, model *domain.AssessmentModel) (SnapshotBuildResult, error)
}

// Registry resolves family handlers by model identity.
type Registry struct {
	handlers []Handler
}

func NewRegistry(handlers ...Handler) Registry {
	copied := make([]Handler, 0, len(handlers))
	for _, handler := range handlers {
		if handler != nil {
			copied = append(copied, handler)
		}
	}
	return Registry{handlers: copied}
}

func (r Registry) Resolve(identity domain.Identity) (Handler, bool) {
	for _, handler := range r.handlers {
		if handler.Supports(identity) {
			return handler, true
		}
	}
	return nil, false
}

func (r Registry) MustResolve(identity domain.Identity) (Handler, error) {
	handler, ok := r.Resolve(identity)
	if ok {
		return handler, nil
	}
	return nil, fmt.Errorf("unsupported assessment model identity %s/%s/%s", identity.Kind, identity.SubKind, identity.Algorithm)
}

// ValidationError keeps structured validation issues visible across
// application orchestration boundaries.
type ValidationError struct {
	Issues []domain.DomainValidationIssue
}

func NewValidationError(issues []domain.DomainValidationIssue) error {
	if len(issues) == 0 {
		return nil
	}
	return &ValidationError{Issues: append([]domain.DomainValidationIssue(nil), issues...)}
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "validation failed"
	}
	return e.Issues[0].Message
}
