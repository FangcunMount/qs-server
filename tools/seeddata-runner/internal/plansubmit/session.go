package plansubmit

import (
	"context"
	"fmt"
	"strings"

	"github.com/FangcunMount/component-base/pkg/log"
)

type planTaskSubmitGateway interface {
	GetPlan(ctx context.Context, planID string) (*PlanResponse, error)
	GetScale(ctx context.Context, code string) (*ScaleResponse, error)
	GetQuestionnaireDetail(ctx context.Context, code string) (*QuestionnaireDetailResponse, error)
	ListPlanTaskWindow(ctx context.Context, req ListPlanTaskWindowRequest) (*PlanTaskWindowResponse, error)
}

type apiPlanTaskSubmitGateway struct {
	client *APIClient
}

func newPlanTaskSubmitGateway(client *APIClient) planTaskSubmitGateway {
	if client == nil {
		return nil
	}
	return &apiPlanTaskSubmitGateway{client: client}
}

func (g *apiPlanTaskSubmitGateway) GetPlan(ctx context.Context, planID string) (*PlanResponse, error) {
	return g.client.GetPlan(ctx, planID)
}

func (g *apiPlanTaskSubmitGateway) GetScale(ctx context.Context, code string) (*ScaleResponse, error) {
	return g.client.GetScale(ctx, code)
}

func (g *apiPlanTaskSubmitGateway) GetQuestionnaireDetail(ctx context.Context, code string) (*QuestionnaireDetailResponse, error) {
	return g.client.GetQuestionnaireDetail(ctx, code)
}

func (g *apiPlanTaskSubmitGateway) ListPlanTaskWindow(ctx context.Context, req ListPlanTaskWindowRequest) (*PlanTaskWindowResponse, error) {
	return g.client.ListPlanTaskWindow(ctx, req)
}

type planTaskSubmitSession struct {
	deps    *dependencies
	logger  log.Logger
	orgID   int64
	planID  string
	gateway planTaskSubmitGateway
	plan    *PlanResponse
}

func openPlanTaskSubmitSession(
	ctx context.Context,
	deps *dependencies,
	planID string,
	verbose bool,
) (*planTaskSubmitSession, error) {
	if deps == nil {
		return nil, fmt.Errorf("dependencies are nil")
	}
	if deps.APIClient == nil {
		return nil, fmt.Errorf("api client is not initialized")
	}

	planID = normalizePlanID(planID)
	if planID == "" {
		return nil, fmt.Errorf("plan-id is required")
	}

	orgID := deps.Config.Global.OrgID
	if orgID <= 0 {
		return nil, fmt.Errorf("global.orgId must be set in seeddata config")
	}

	logger := deps.Logger
	prewarmAPIToken(ctx, deps.APIClient, orgID, logger)

	gateway := newPlanTaskSubmitGateway(deps.APIClient)
	if gateway == nil {
		return nil, fmt.Errorf("initialize opened-task submit gateway: api client is nil")
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

	if verbose {
		logger.Infow("Opened-task submit session initialized",
			"plan_id", planID,
			"org_id", orgID,
			"plan_status", planResp.Status,
			"scale_code", planResp.ScaleCode,
		)
	}

	return &planTaskSubmitSession{
		deps:    deps,
		logger:  logger,
		orgID:   orgID,
		planID:  planID,
		gateway: gateway,
		plan:    planResp,
	}, nil
}

func loadPlanTaskSubmitQuestionnaire(
	ctx context.Context,
	session *planTaskSubmitSession,
	verbose bool,
) (*ScaleResponse, *QuestionnaireDetailResponse, error) {
	if session == nil {
		return nil, nil, fmt.Errorf("plan task submit session is nil")
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
