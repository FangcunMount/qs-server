package cachegovernance

import (
	"context"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
)

func (c *coordinator) HandleRepairComplete(ctx context.Context, req RepairCompleteRequest) error {
	if c == nil || !c.cfg.Enable {
		return nil
	}

	var targets []cachetarget.WarmupTarget
	switch strings.TrimSpace(req.RepairKind) {
	case "statistics_backfill":
		targets = append(targets, c.repairQueryTargets(req)...)
		targets = append(targets, c.mergeQueryTargets(ctx, req.OrgIDs, &req)...)
	case "journey_rebuild_history":
		targets = append(targets, c.repairQueryTargets(req)...)
	default:
		targets = append(targets, c.repairQueryTargets(req)...)
	}
	_, err := c.executeTargets(ctx, "repair", targets)
	return err
}

func (c *coordinator) repairQueryTargets(req RepairCompleteRequest) []cachetarget.WarmupTarget {
	if len(req.OrgIDs) == 0 {
		return nil
	}
	targets := make([]cachetarget.WarmupTarget, 0)
	for _, orgID := range req.OrgIDs {
		if orgID <= 0 {
			continue
		}
		if strings.TrimSpace(req.RepairKind) == "statistics_backfill" {
			for _, preset := range overviewSeedPresets(nil) {
				targets = append(targets, cachetarget.NewQueryStatsOverviewWarmupTarget(orgID, preset))
			}
			targets = append(targets, cachetarget.NewQueryStatsSystemWarmupTarget(orgID))
		}
		for _, code := range req.QuestionnaireCodes {
			targets = append(targets, cachetarget.NewQueryStatsQuestionnaireWarmupTarget(orgID, code))
		}
		for _, planID := range req.PlanIDs {
			targets = append(targets, cachetarget.NewQueryStatsPlanWarmupTarget(orgID, planID))
		}
	}
	return dedupeTargets(targets)
}
