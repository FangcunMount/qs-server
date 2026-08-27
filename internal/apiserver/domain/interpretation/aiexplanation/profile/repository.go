package profile

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type Repository interface {
	Save(context.Context, *AIExplanationProfile) error
	FindByKey(context.Context, string, string) (*AIExplanationProfile, error)
	ListPublishedByBaseSelector(context.Context, policy.Audience, modelcatalog.Kind, modelcatalog.DecisionKind) ([]*AIExplanationProfile, error)
}
