package cachegovernance

import (
	"context"
	"strings"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
)

func (c *coordinator) HandleScalePublished(ctx context.Context, code string) error {
	if c == nil || !c.cfg.Enable {
		return nil
	}
	targets := []cachetarget.WarmupTarget{
		cachetarget.NewStaticScaleWarmupTarget(code),
	}
	if c.deps.LookupScaleQuestionnaireCode != nil {
		if questionnaireCode, err := c.deps.LookupScaleQuestionnaireCode(ctx, code); err != nil {
			logger.L(ctx).Warnw("failed to resolve questionnaire linked to scale during publish warmup",
				"scale_code", code,
				"error", err,
			)
		} else if questionnaireCode != "" {
			targets = append(targets, cachetarget.NewStaticQuestionnaireWarmupTarget(questionnaireCode))
		}
	}
	_, err := c.executeTargets(ctx, "publish", targets)
	return err
}

func (c *coordinator) HandleQuestionnairePublished(ctx context.Context, code, _ string) error {
	if c == nil || !c.cfg.Enable {
		return nil
	}
	_, err := c.executeTargets(ctx, "publish", []cachetarget.WarmupTarget{
		cachetarget.NewStaticQuestionnaireWarmupTarget(code),
	})
	return err
}

func (c *coordinator) HandleTypologyModelPublished(ctx context.Context, code string) error {
	if c == nil || !c.cfg.Enable || strings.TrimSpace(code) == "" {
		return nil
	}
	_, err := c.executeTargets(ctx, "publish", []cachetarget.WarmupTarget{
		cachetarget.NewStaticTypologyModelWarmupTarget(code),
	})
	return err
}

func (c *coordinator) HandleStatisticsSync(ctx context.Context, orgID int64) error {
	if c == nil || !c.cfg.Enable || orgID <= 0 {
		return nil
	}
	targets := []cachetarget.WarmupTarget{
		cachetarget.NewQueryStatsOverviewWarmupTarget(orgID, "latest_complete_day"),
		cachetarget.NewQueryStatsOverviewWarmupTarget(orgID, "7d"),
		cachetarget.NewQueryStatsOverviewWarmupTarget(orgID, "30d"),
	}
	_, err := c.executeTargets(ctx, "statistics_sync", append(targets, c.mergeQueryTargets(ctx, []int64{orgID}, nil)...))
	return err
}
