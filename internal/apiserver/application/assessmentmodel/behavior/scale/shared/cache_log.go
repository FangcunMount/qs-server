package shared

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
)

// LogScaleListCacheError logs list-cache rebuild failures without failing the caller.
func LogScaleListCacheError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	logger.L(ctx).Warnw("failed to rebuild scale list cache", "error", err)
}
