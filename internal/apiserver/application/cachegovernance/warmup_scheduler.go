package cachegovernance

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
)

func (c *coordinator) WarmStartup(ctx context.Context) error {
	if c == nil || !c.cfg.Enable {
		return nil
	}
	targets := make([]cachetarget.WarmupTarget, 0)
	if c.cfg.StartupStatic {
		targets = append(targets, c.startupStaticTargets(ctx)...)
	}
	if c.cfg.StartupQuery {
		targets = append(targets, c.mergeQueryTargets(ctx, nil, nil)...)
	}
	if c.deps.StatisticsSeeds != nil && c.deps.StatisticsSeeds.WarmOnStartup {
		targets = append(targets, c.planner().querySeedTargets(nil)...)
	}
	_, err := c.executeTargets(ctx, "startup", dedupeTargets(targets))
	return err
}

func (c *coordinator) startupStaticTargets(ctx context.Context) []cachetarget.WarmupTarget {
	targets := make([]cachetarget.WarmupTarget, 0)
	if c.deps.ListPublishedScaleCodes != nil {
		if codes, err := c.deps.ListPublishedScaleCodes(ctx); err != nil {
			logger.L(ctx).Warnw("failed to load published scales for startup warmup", "error", err)
		} else {
			for _, code := range codes {
				targets = append(targets, cachetarget.NewStaticScaleWarmupTarget(code))
			}
		}
	}
	if c.deps.ListPublishedQuestionnaireCodes != nil {
		if codes, err := c.deps.ListPublishedQuestionnaireCodes(ctx); err != nil {
			logger.L(ctx).Warnw("failed to load published questionnaires for startup warmup", "error", err)
		} else {
			for _, code := range codes {
				targets = append(targets, cachetarget.NewStaticQuestionnaireWarmupTarget(code))
			}
		}
	}
	return dedupeTargets(targets)
}
