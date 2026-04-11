package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
)

type planSeedSession struct {
	ctx            context.Context
	deps           *dependencies
	logger         log.Logger
	orgID          int64
	planID         string
	gateway        PlanSeedGateway
	cleanupGateway func() error
	plan           *PlanResponse
}

func openPlanSeedSession(
	ctx context.Context,
	deps *dependencies,
	planID string,
	verbose bool,
) (*planSeedSession, error) {
	if deps == nil {
		return nil, fmt.Errorf("dependencies are nil")
	}
	if deps.APIClient == nil {
		return nil, fmt.Errorf("api client is not initialized")
	}
	orgID := deps.Config.Global.OrgID
	if orgID <= 0 {
		return nil, fmt.Errorf("global.orgId must be set in seeddata config")
	}
	planID = normalizePlanID(planID)

	logger := deps.Logger
	ctx = withSeedPlanPacer(
		ctx,
		newSeedPlanPacer(
			time.Now(),
			seedPlanPaceInterval,
			seedPlanPaceSleep,
			logger,
			verbose,
		),
	)
	prewarmAPIToken(ctx, deps.APIClient, orgID, logger)

	gateway, cleanupGateway, err := newPlanSeedGateway(ctx, deps, !verbose)
	if err != nil {
		return nil, fmt.Errorf("initialize local plan seed gateway: %w", err)
	}

	closeOnError := func() {
		if cleanupGateway != nil {
			if cleanupErr := cleanupGateway(); cleanupErr != nil {
				logger.Warnw("Failed to cleanup plan seed gateway",
					"plan_id", planID,
					"error", cleanupErr.Error(),
				)
			}
		}
	}

	planResp, err := gateway.GetPlan(ctx, planID)
	if err != nil {
		closeOnError()
		return nil, fmt.Errorf("load plan %s from local runtime: %w; seeddata plan steps read plan data from --local-mysql-dsn/--local-mongo-uri/--local-redis-* instead of %s, so verify those local connections point to the environment that contains this plan", planID, err, strings.TrimSpace(deps.APIClient.baseURL))
	}
	if planResp == nil {
		closeOnError()
		return nil, fmt.Errorf("plan %s not found", planID)
	}
	if planResp.OrgID != orgID {
		closeOnError()
		return nil, fmt.Errorf("plan %s does not belong to org %d", planID, orgID)
	}
	if normalizeTaskStatus(planResp.Status) != "active" {
		closeOnError()
		return nil, fmt.Errorf("plan %s is not active, current status=%s", planID, planResp.Status)
	}

	return &planSeedSession{
		ctx:            ctx,
		deps:           deps,
		logger:         logger,
		orgID:          orgID,
		planID:         planID,
		gateway:        gateway,
		cleanupGateway: cleanupGateway,
		plan:           planResp,
	}, nil
}

func (s *planSeedSession) Close() {
	if s == nil || s.cleanupGateway == nil {
		return
	}
	if err := s.cleanupGateway(); err != nil {
		s.logger.Warnw("Failed to cleanup plan seed gateway",
			"plan_id", s.planID,
			"error", err.Error(),
		)
	}
}
