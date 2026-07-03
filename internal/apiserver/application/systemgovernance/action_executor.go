package systemgovernance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ActionExecutor runs enabled governance actions.
type ActionExecutor struct {
	registry   *ActionRegistry
	governance statisticsApp.GovernanceFacade
}

// NewActionExecutor creates an action executor.
func NewActionExecutor(registry *ActionRegistry, governance statisticsApp.GovernanceFacade) *ActionExecutor {
	return &ActionExecutor{registry: registry, governance: governance}
}

// Run executes one enabled action.
func (e *ActionExecutor) Run(
	ctx context.Context,
	orgID int64,
	actionID string,
	req ActionRunRequest,
) (*ActionRunResult, error) {
	descriptor, ok := e.registry.Get(actionID)
	if !ok {
		return nil, errors.WithCode(code.ErrInvalidArgument, "unknown action: %s", actionID)
	}
	if !descriptor.Enabled {
		return nil, errors.WithCode(code.ErrInvalidArgument, "action %s is not enabled", actionID)
	}
	if descriptor.RequiresConfirmation && !req.Confirm {
		return nil, errors.WithCode(code.ErrInvalidArgument, "action %s requires confirm=true", actionID)
	}
	startedAt := time.Now()
	switch actionID {
	case "cache.manual_warmup":
		result, err := e.runManualWarmup(ctx, orgID, req.Input)
		return finalizeActionRun(actionID, startedAt, result, err)
	case "cache.repair_complete":
		result, err := e.runRepairComplete(ctx, orgID, req.Input)
		return finalizeActionRun(actionID, startedAt, result, err)
	default:
		return nil, errors.WithCode(code.ErrInvalidArgument, "action %s is not executable in v1", actionID)
	}
}

func (e *ActionExecutor) runManualWarmup(ctx context.Context, orgID int64, input map[string]interface{}) (map[string]interface{}, error) {
	if e == nil || e.governance == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "cache governance facade unavailable")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "invalid input: %s", err.Error())
	}
	var req statisticsApp.ManualWarmupRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "invalid manual warmup input: %s", err.Error())
	}
	result, err := e.governance.HandleManualWarmup(ctx, orgID, req)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"warmup": result}, nil
}

func (e *ActionExecutor) runRepairComplete(ctx context.Context, orgID int64, input map[string]interface{}) (map[string]interface{}, error) {
	if e == nil || e.governance == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "cache governance facade unavailable")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "invalid input: %s", err.Error())
	}
	var req statisticsApp.RepairCompleteRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "invalid repair complete input: %s", err.Error())
	}
	if err := e.governance.HandleRepairComplete(ctx, orgID, req); err != nil {
		return nil, err
	}
	return map[string]interface{}{"message": "repair complete hook accepted"}, nil
}

func finalizeActionRun(actionID string, startedAt time.Time, result map[string]interface{}, err error) (*ActionRunResult, error) {
	if err != nil {
		return nil, err
	}
	return &ActionRunResult{
		ActionID:   actionID,
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		Status:     "ok",
		Message:    fmt.Sprintf("action %s completed", actionID),
		Result:     result,
	}, nil
}
