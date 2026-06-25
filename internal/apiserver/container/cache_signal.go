package container

import (
	"context"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
)

func (c *Container) initCacheSignalNotifier() error {
	if c == nil {
		return nil
	}
	notifier, err := cachesignal.NewNotifier(
		c.CacheHandle(cacheplane.FamilyOps),
		cachesignal.ConfigFromReportStatus(c.reportStatusConfig),
	)
	if err != nil {
		return err
	}
	c.cacheSignalNotifier = notifier
	return nil
}

func (c *Container) CacheSignalNotifier() *cachesignal.Notifier {
	if c == nil {
		return nil
	}
	return c.cacheSignalNotifier
}

func (c *Container) StartCacheSignalWatcher(ctx context.Context) {
	if c == nil {
		return
	}
	notifier := c.CacheSignalNotifier()
	if notifier == nil {
		return
	}
	cachegov.StartCacheSignalWatcher(
		ctx,
		c.WarmupCoordinator(),
		notifier.QuestionnaireSignaler(),
		notifier.ScaleSignaler(),
	)
}
