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
	planMode       string
	gateway        PlanSeedGateway
	cleanupGateway func() error
	plan           *PlanResponse
}

func openPlanSeedSession(
	ctx context.Context,
	deps *dependencies,
	planID string,
	planMode string,
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

	gateway, cleanupGateway, err := newPlanSeedGateway(ctx, deps, planMode, !verbose)
	if err != nil {
		return nil, fmt.Errorf("initialize plan seed gateway (%s): %w", planMode, err)
	}

	closeOnError := func() {
		if cleanupGateway != nil {
			if cleanupErr := cleanupGateway(); cleanupErr != nil {
				logger.Warnw("Failed to cleanup plan seed gateway",
					"plan_id", planID,
					"plan_mode", planMode,
					"error", cleanupErr.Error(),
				)
			}
		}
	}

	planResp, err := gateway.GetPlan(ctx, planID)
	if err != nil {
		closeOnError()
		if planMode == planModeLocal {
			return nil, fmt.Errorf("load plan %s in local mode: %w; local mode reads plan data from --local-mysql-dsn/--local-mongo-uri/--local-redis-* instead of %s, so verify those local connections point to the environment that contains this plan or rerun with --plan-mode remote", planID, err, strings.TrimSpace(deps.APIClient.baseURL))
		}
		return nil, fmt.Errorf("load plan %s: %w", planID, err)
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
		planMode:       planMode,
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
			"plan_mode", s.planMode,
			"error", err.Error(),
		)
	}
}

func loadPlanProcessQuestionnaire(
	ctx context.Context,
	session *planSeedSession,
	verbose bool,
) (*ScaleResponse, *QuestionnaireDetailResponse, error) {
	if session == nil {
		return nil, nil, fmt.Errorf("plan seed session is nil")
	}
	if session.plan == nil {
		return nil, nil, fmt.Errorf("plan is nil")
	}
	if strings.TrimSpace(session.plan.ScaleCode) == "" {
		return nil, nil, fmt.Errorf("plan %s has empty scale_code", session.planID)
	}

	scaleResp, err := session.gateway.GetScale(ctx, session.plan.ScaleCode)
	if err != nil {
		return nil, nil, fmt.Errorf("load scale %s: %w", session.plan.ScaleCode, err)
	}
	if scaleResp == nil {
		return nil, nil, fmt.Errorf("scale %s not found", session.plan.ScaleCode)
	}
	if strings.TrimSpace(scaleResp.QuestionnaireCode) == "" {
		return nil, nil, fmt.Errorf("scale %s has empty questionnaire_code", session.plan.ScaleCode)
	}
	if strings.TrimSpace(scaleResp.QuestionnaireVersion) == "" {
		return nil, nil, fmt.Errorf("scale %s has empty questionnaire_version", session.plan.ScaleCode)
	}

	detail, err := session.gateway.GetQuestionnaireDetail(ctx, scaleResp.QuestionnaireCode)
	if err != nil {
		return nil, nil, fmt.Errorf("load questionnaire %s: %w", scaleResp.QuestionnaireCode, err)
	}
	if detail == nil {
		return nil, nil, fmt.Errorf("questionnaire %s not found", scaleResp.QuestionnaireCode)
	}
	if strings.TrimSpace(detail.Version) != scaleResp.QuestionnaireVersion {
		return nil, nil, newPlanQuestionnaireVersionMismatchError(
			session.plan.ScaleCode,
			scaleResp.QuestionnaireCode,
			scaleResp.QuestionnaireVersion,
			detail.Version,
		)
	}
	if verbose {
		debugLogQuestionnaire(detail, session.logger)
	}

	return scaleResp, detail, nil
}
