package statistics

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type governanceFacade struct {
	component     string
	coordinator   cachegov.Coordinator
	statusService cachegov.StatusService
}

// NewGovernanceFacade 创建统计治理 facade。
func NewGovernanceFacade(component string, coordinator cachegov.Coordinator, statusService cachegov.StatusService) GovernanceFacade {
	return &governanceFacade{
		component:     component,
		coordinator:   coordinator,
		statusService: statusService,
	}
}

func (f *governanceFacade) TriggerStatisticsWarmup(ctx context.Context, orgID int64, action string) {
	if f == nil || f.coordinator == nil {
		return
	}
	if err := f.coordinator.HandleStatisticsSync(ctx, orgID); err != nil {
		logger.L(ctx).Warnw("statistics sync cache governance hook failed",
			"action", action,
			"org_id", orgID,
			"error", err,
		)
	}
}

func (f *governanceFacade) HandleRepairComplete(ctx context.Context, protectedOrgID int64, req RepairCompleteRequest) error {
	normalized, err := normalizeRepairCompleteRequest(protectedOrgID, req)
	if err != nil {
		return err
	}
	if f == nil || f.coordinator == nil {
		return nil
	}
	if err := f.coordinator.HandleRepairComplete(ctx, normalized); err != nil {
		logger.L(ctx).Warnw("repair-complete cache governance hook failed",
			"repair_kind", normalized.RepairKind,
			"org_ids", normalized.OrgIDs,
			"error", err,
		)
	}
	return nil
}

func (f *governanceFacade) HandleManualWarmup(ctx context.Context, protectedOrgID int64, req ManualWarmupRequest) (*cachegov.ManualWarmupResult, error) {
	if err := validateManualWarmupTargets(protectedOrgID, req.Targets); err != nil {
		return nil, err
	}
	if f == nil || f.coordinator == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "warmup coordinator is unavailable")
	}

	result, err := f.coordinator.HandleManualWarmup(ctx, req)
	if err != nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "%s", err.Error())
	}
	return result, nil
}

func (f *governanceFacade) GetStatus(ctx context.Context) (*cachegov.StatusSnapshot, error) {
	if f == nil || f.statusService == nil {
		return &cachegov.StatusSnapshot{
			RuntimeSnapshot: cacheobservability.RuntimeSnapshot{
				GeneratedAt: time.Now(),
				Component:   f.componentName(),
				Families:    []cacheobservability.FamilyStatus{},
				Summary: cacheobservability.RuntimeSummary{
					Ready: true,
				},
			},
			Warmup: cachegov.WarmupStatusSnapshot{},
		}, nil
	}
	return f.statusService.GetStatus(ctx)
}

func (f *governanceFacade) GetHotset(ctx context.Context, kindRaw, limitRaw string) (*GovernanceHotsetResponse, error) {
	if f == nil || f.statusService == nil {
		return &GovernanceHotsetResponse{
			Items:     []cachetarget.HotsetItem{},
			Available: false,
			Degraded:  true,
			Message:   "cache governance status service unavailable",
		}, nil
	}

	kind, err := parseWarmupKind(kindRaw)
	if err != nil {
		return nil, err
	}
	limit, err := parseHotsetLimit(limitRaw)
	if err != nil {
		return nil, err
	}

	result, err := f.statusService.GetHotset(ctx, kind, limit)
	if err != nil {
		return nil, err
	}
	return &GovernanceHotsetResponse{
		Family:    result.Family,
		Kind:      result.Kind,
		Limit:     result.Limit,
		Available: result.Available,
		Degraded:  result.Degraded,
		Message:   result.Message,
		Items:     result.Items,
	}, nil
}

func (f *governanceFacade) componentName() string {
	if f != nil && f.component != "" {
		return f.component
	}
	return "apiserver"
}

func normalizeRepairCompleteRequest(protectedOrgID int64, req RepairCompleteRequest) (cachegov.RepairCompleteRequest, error) {
	orgIDs := req.OrgIDs
	if len(orgIDs) == 0 {
		orgIDs = []int64{protectedOrgID}
	}
	for _, candidate := range orgIDs {
		if candidate != protectedOrgID {
			return cachegov.RepairCompleteRequest{}, errors.WithCode(code.ErrInvalidArgument, "org_ids must stay within the protected org scope")
		}
	}
	return cachegov.RepairCompleteRequest{
		RepairKind:         req.RepairKind,
		OrgIDs:             orgIDs,
		QuestionnaireCodes: req.QuestionnaireCodes,
		PlanIDs:            req.PlanIDs,
	}, nil
}

func validateManualWarmupTargets(protectedOrgID int64, targets []cachegov.ManualWarmupTarget) error {
	if len(targets) == 0 {
		return errors.WithCode(code.ErrInvalidArgument, "targets cannot be empty")
	}
	for _, item := range targets {
		target, err := cachegov.ParseManualWarmupTarget(item)
		if err != nil {
			return errors.WithCode(code.ErrInvalidArgument, "%s", err.Error())
		}
		if orgID, ok := cachegov.WarmupTargetOrgID(target); ok && orgID != protectedOrgID {
			return errors.WithCode(code.ErrInvalidArgument, "query warmup target org must stay within the protected org scope")
		}
	}
	return nil
}

func parseWarmupKind(raw string) (cachetarget.WarmupKind, error) {
	kind, ok := cachetarget.ParseWarmupKind(strings.TrimSpace(raw))
	if !ok {
		return "", errors.WithCode(code.ErrInvalidArgument, "invalid kind")
	}
	return kind, nil
}

func parseHotsetLimit(raw string) (int64, error) {
	if strings.TrimSpace(raw) == "" {
		return 20, nil
	}
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0, errors.WithCode(code.ErrInvalidArgument, "invalid limit")
	}
	if value > 100 {
		value = 100
	}
	return value, nil
}
