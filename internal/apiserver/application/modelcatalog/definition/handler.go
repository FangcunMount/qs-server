package definition

import (
	"context"
	"encoding/json"
	"fmt"

	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

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
// Family handlers validate canonical DefinitionV2 when supplied; legacy wire
// import belongs at the owning API adapter boundary.
type Handler interface {
	Supports(identity domain.Identity) bool
	ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue
	BuildSnapshotPayload(ctx context.Context, model *domain.AssessmentModel) (SnapshotBuildResult, error)
}

// PreviewResult is the strategy-owned representation of a definition report
// preview. The authoring use case owns its transport projection.
type PreviewResult struct {
	OutcomeCode    string
	OutcomeTitle   string
	ScoreDetail    map[string]float64
	ReportSections []PreviewSection
	RawReport      *report.InterpretReport
}

type PreviewSection struct {
	Title   string
	Content string
	Kind    string
}

// PreviewHandler is implemented only by definition strategies that support
// report preview. It keeps family-specific execution shaping inside Registry.
type PreviewHandler interface {
	PreviewReport(context.Context, *domain.AssessmentModel, json.RawMessage) (*PreviewResult, error)
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

func (r Registry) PreviewReport(ctx context.Context, model *domain.AssessmentModel, input json.RawMessage) (*PreviewResult, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	handler, err := r.MustResolve(domain.Identity{Kind: model.Kind, SubKind: model.SubKind, Algorithm: model.Algorithm})
	if err != nil {
		return nil, err
	}
	preview, ok := handler.(PreviewHandler)
	if !ok {
		return nil, fmt.Errorf("report preview is not configured for model identity %s/%s/%s", model.Kind, model.SubKind, model.Algorithm)
	}
	return preview.PreviewReport(ctx, model, input)
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
