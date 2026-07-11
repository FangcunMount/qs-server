package report

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ArtifactRepository stores successful immutable report artifacts only.
// Implementations must enforce one artifact per Generation.
type ArtifactRepository interface {
	Insert(ctx context.Context, artifact *Artifact) error
	FindByID(ctx context.Context, id meta.ID) (*Artifact, error)
	FindByGenerationID(ctx context.Context, generationID meta.ID) (*Artifact, error)
	FindLatestByAssessmentID(ctx context.Context, assessmentID meta.ID) (*Artifact, error)
	ListByAssessmentID(ctx context.Context, assessmentID meta.ID) ([]*Artifact, error)
}
