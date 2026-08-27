package artifact

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Repository stores validated immutable AI explanation artifacts only.
// Implementations must enforce one artifact per Generation.
type Repository interface {
	Insert(context.Context, *AIExplanationArtifact) error
	FindByID(context.Context, meta.ID) (*AIExplanationArtifact, error)
	FindByGenerationID(context.Context, meta.ID) (*AIExplanationArtifact, error)
	FindBySourceReportAndAudience(context.Context, meta.ID, policy.Audience) (*AIExplanationArtifact, error)
}
