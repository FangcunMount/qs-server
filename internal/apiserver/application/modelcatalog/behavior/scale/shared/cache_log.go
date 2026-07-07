package shared

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
)

// LogScaleListCacheError logs list-缓存 rebuild 失败 不使用 failing caller。
func LogScaleListCacheError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	logger.L(ctx).Warnw("failed to rebuild scale list cache", "error", err)
}
