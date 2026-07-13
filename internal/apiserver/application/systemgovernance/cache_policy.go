package systemgovernance

import (
	"context"

	cachemodel "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
)

// CachePolicyReloader is the application-owned port for publishing a validated
// cache policy snapshot. Configuration parsing remains in process bootstrap.
type CachePolicyReloader interface {
	ReloadPolicy(context.Context, int64, cachemodel.CachePolicyReloadRequest) (*cachemodel.CachePolicyReloadResult, error)
}
