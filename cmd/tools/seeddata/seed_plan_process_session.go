package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
)

type planProcessGateway interface {
	GetPlan(ctx context.Context, planID string) (*PlanResponse, error)
	GetScale(ctx context.Context, code string) (*ScaleResponse, error)
	GetQuestionnaireDetail(ctx context.Context, code string) (*QuestionnaireDetailResponse, error)
	SchedulePendingTasks(ctx context.Context, req SchedulePendingTasksRequest) (*TaskListResponse, error)
	ListPlanTaskWindow(ctx context.Context, req ListPlanTaskWindowRequest) (*PlanTaskWindowResponse, error)
	GetTask(ctx context.Context, taskID string) (*TaskResponse, error)
	ExpireTask(ctx context.Context, taskID string) (*TaskResponse, error)
}

type apiPlanProcessGateway struct {
	client *APIClient
}

func newPlanProcessGateway(client *APIClient) planProcessGateway {
	if client == nil {
		return nil
	}
	return &apiPlanProcessGateway{client: client}
}

func (g *apiPlanProcessGateway) GetPlan(ctx context.Context, planID string) (*PlanResponse, error) {
	return g.client.GetPlan(ctx, planID)
}

func (g *apiPlanProcessGateway) GetScale(ctx context.Context, code string) (*ScaleResponse, error) {
	return g.client.GetScale(ctx, code)
}

func (g *apiPlanProcessGateway) GetQuestionnaireDetail(ctx context.Context, code string) (*QuestionnaireDetailResponse, error) {
	return g.client.GetQuestionnaireDetail(ctx, code)
}

func (g *apiPlanProcessGateway) SchedulePendingTasks(ctx context.Context, req SchedulePendingTasksRequest) (*TaskListResponse, error) {
	return g.client.SchedulePendingTasks(ctx, req)
}

func (g *apiPlanProcessGateway) ListPlanTaskWindow(ctx context.Context, req ListPlanTaskWindowRequest) (*PlanTaskWindowResponse, error) {
	return g.client.ListPlanTaskWindow(ctx, req)
}

func (g *apiPlanProcessGateway) GetTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	return g.client.GetTask(ctx, taskID)
}

func (g *apiPlanProcessGateway) ExpireTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	return g.client.ExpireTask(ctx, taskID)
}

type planProcessSession struct {
	ctx     context.Context
	deps    *dependencies
	logger  log.Logger
	orgID   int64
	planID  string
	gateway planProcessGateway
	plan    *PlanResponse
}

func openPlanProcessSession(
	ctx context.Context,
	deps *dependencies,
	planID string,
	verbose bool,
) (*planProcessSession, error) {
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

	gateway := newPlanProcessGateway(deps.APIClient)
	if gateway == nil {
		return nil, fmt.Errorf("initialize plan process gateway: api client is nil")
	}

	planResp, err := gateway.GetPlan(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("load plan %s from apiserver api: %w", planID, err)
	}
	if planResp == nil {
		return nil, fmt.Errorf("plan %s not found", planID)
	}
	if planResp.OrgID != orgID {
		return nil, fmt.Errorf("plan %s does not belong to org %d", planID, orgID)
	}
	if normalizeTaskStatus(planResp.Status) != "active" {
		return nil, fmt.Errorf("plan %s is not active, current status=%s", planID, planResp.Status)
	}

	return &planProcessSession{
		ctx:     ctx,
		deps:    deps,
		logger:  logger,
		orgID:   orgID,
		planID:  planID,
		gateway: gateway,
		plan:    planResp,
	}, nil
}

func (s *planProcessSession) Close() {}

func loadPlanProcessQuestionnaire(
	ctx context.Context,
	session *planProcessSession,
	verbose bool,
) (*ScaleResponse, *QuestionnaireDetailResponse, error) {
	if session == nil {
		return nil, nil, fmt.Errorf("plan process session is nil")
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
