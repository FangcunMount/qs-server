package systemgovernance

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	cachemodel "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
	uuid "github.com/satori/go.uuid"
)

// ActionExecutor 运行enabled governance actions。
type ActionExecutor struct {
	registry   *ActionRegistry
	governance statisticsApp.GovernanceFacade
	reloader   CachePolicyReloader
	resilience control.Governor
	audit      ActionAuditStore
}

func NewActionExecutorWithResilience(registry *ActionRegistry, governance statisticsApp.GovernanceFacade, reloader CachePolicyReloader, resilience control.Governor, audits ...ActionAuditStore) *ActionExecutor {
	executor := &ActionExecutor{registry: registry, governance: governance, reloader: reloader, resilience: resilience}
	if len(audits) > 0 {
		executor.audit = audits[0]
	}
	return executor
}

// NewActionExecutor 创建action executor。
func NewActionExecutor(registry *ActionRegistry, governance statisticsApp.GovernanceFacade, reloaders ...CachePolicyReloader) *ActionExecutor {
	executor := &ActionExecutor{registry: registry, governance: governance}
	if len(reloaders) > 0 {
		executor.reloader = reloaders[0]
	}
	return executor
}

// Run 执行一个enabled action。
func (e *ActionExecutor) Run(
	ctx context.Context,
	orgID int64,
	actionID string,
	req ActionRunRequest,
) (runResult *ActionRunResult, runErr error) {
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
	requestID := req.RequestID
	if requestID == "" {
		requestID = uuid.Must(uuid.NewV4(), nil).String()
	}
	audit := newActionAuditRecord(ctx, orgID, requestID, actionID, req.Input, startedAt)
	if e.audit != nil {
		existing, claimed, err := e.audit.Claim(ctx, audit)
		if err != nil {
			return nil, errors.WithCode(code.ErrInternalServerError, "claim governance audit: %s", err.Error())
		}
		if existing != nil {
			if existing.ActionID != "" && existing.ActionID != actionID {
				return nil, errors.WithCode(code.ErrConflict, "request_id already belongs to action %s", existing.ActionID)
			}
			if existing.Error != nil {
				return nil, errors.WithCode(existing.Error.Code, "%s", existing.Error.Message)
			}
			return existing.Result, nil
		}
		if !claimed {
			return nil, errors.WithCode(code.ErrConflict, "request_id is already running")
		}
		defer func() {
			audit.FinishedAt = time.Now()
			audit.Status = actionAuditStatus(runResult, runErr)
			audit.Result = runResult
			if runErr != nil {
				coder := errors.ParseCoder(runErr)
				audit.Error = &ActionAuditError{Code: coder.Code(), Message: runErr.Error()}
			} else if audit.Result == nil {
				audit.Result = &ActionRunResult{
					RequestID: requestID, ActionID: actionID, StartedAt: startedAt,
					FinishedAt: audit.FinishedAt, Status: audit.Status,
				}
			}
			auditCtx, cancelAudit := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Second)
			defer cancelAudit()
			if err := e.audit.Complete(auditCtx, audit); err != nil {
				runResult = nil
				runErr = errors.WithCode(code.ErrInternalServerError, "governance action outcome could not be persisted")
			}
		}()
	}
	switch actionID {
	case "cache.manual_warmup":
		result, err := e.runManualWarmup(ctx, orgID, req.Input)
		return finalizeActionRun(requestID, actionID, startedAt, result, err)
	case "cache.repair_complete":
		result, err := e.runRepairComplete(ctx, orgID, req.Input)
		return finalizeActionRun(requestID, actionID, startedAt, result, err)
	case "cache.reload_policy":
		result, err := e.runReloadPolicy(ctx, orgID, req.Input)
		return finalizeActionRun(requestID, actionID, startedAt, result, err)
	case "resilience.tune_rate_limit":
		result, err := e.runTuneRateLimit(ctx, orgID, req.Input)
		return finalizeActionRun(requestID, actionID, startedAt, result, err)
	case "resilience.drain_queue":
		result, err := e.runQueueState(ctx, orgID, requestID, req.Input, control.QueueStatePaused)
		return finalizeActionRun(requestID, actionID, startedAt, result, err)
	case "resilience.resume_queue":
		result, err := e.runQueueState(ctx, orgID, requestID, req.Input, control.QueueStateActive)
		return finalizeActionRun(requestID, actionID, startedAt, result, err)
	case "resilience.release_lock":
		result, err := e.runReleaseLock(ctx, orgID, requestID, req.Input)
		return finalizeActionRun(requestID, actionID, startedAt, result, err)
	default:
		return nil, errors.WithCode(code.ErrInvalidArgument, "action %s is not executable in v1", actionID)
	}
}

func newActionAuditRecord(ctx context.Context, orgID int64, requestID, actionID string, input map[string]interface{}, startedAt time.Time) ActionAuditRecord {
	component, _ := input["component"].(string)
	instanceID, _ := input["instance_id"].(string)
	if instanceID == "" {
		instanceID, _ = input["target"].(string)
	}
	return ActionAuditRecord{
		RequestID: requestID, ActionID: actionID, OrgID: orgID,
		ActorUserID: actorctx.GrantingUserID(ctx), Component: component,
		TargetInstance: instanceID, Input: redactActionInput(input), StartedAt: startedAt,
	}
}

func redactActionInput(input map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(input))
	for key, value := range input {
		normalized := strings.ToLower(key)
		switch {
		case strings.Contains(normalized, "password"), strings.Contains(normalized, "secret"),
			strings.Contains(normalized, "token"), strings.Contains(normalized, "redis_key"):
			result[key] = "[REDACTED]"
		default:
			result[key] = redactActionValue(value)
		}
	}
	return result
}

func redactActionValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return redactActionInput(typed)
	case []interface{}:
		result := make([]interface{}, len(typed))
		for index, item := range typed {
			result[index] = redactActionValue(item)
		}
		return result
	default:
		return value
	}
}

func actionAuditStatus(result *ActionRunResult, err error) string {
	if result != nil && result.Status != "" {
		return result.Status
	}
	if err != nil {
		if stderrors.Is(err, context.DeadlineExceeded) {
			return string(control.CommandStatusTimeout)
		}
		return string(control.CommandStatusFailed)
	}
	return string(control.CommandStatusOK)
}

func (e *ActionExecutor) runTuneRateLimit(ctx context.Context, orgID int64, input map[string]interface{}) (map[string]interface{}, error) {
	if e == nil || e.resilience == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "resilience governance unavailable")
	}
	var change control.RateLimitChange
	if err := decodeActionInput(input, &change); err != nil {
		return nil, err
	}
	result, err := e.resilience.TuneRateLimit(ctx, actionActor(ctx, orgID), change)
	return actionResultMap(result), normalizeResilienceError(err)
}

func (e *ActionExecutor) runQueueState(ctx context.Context, orgID int64, requestID string, input map[string]interface{}, desired control.QueueState) (map[string]interface{}, error) {
	if e == nil || e.resilience == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "resilience governance unavailable")
	}
	var change control.QueueChange
	if err := decodeActionInput(input, &change); err != nil {
		return nil, err
	}
	change.DesiredState = desired
	change.RequestID = requestID
	result, err := e.resilience.SetQueueState(ctx, actionActor(ctx, orgID), change)
	return actionResultMap(result), normalizeResilienceError(err)
}

func (e *ActionExecutor) runReleaseLock(ctx context.Context, orgID int64, requestID string, input map[string]interface{}) (map[string]interface{}, error) {
	if e == nil || e.resilience == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "resilience governance unavailable")
	}
	var change control.LeaderChange
	if err := decodeActionInput(input, &change); err != nil {
		return nil, err
	}
	change.RequestID = requestID
	result, err := e.resilience.RelinquishLeader(ctx, actionActor(ctx, orgID), change)
	view := actionResultMap(result)
	if err == nil {
		if _, exists := view["status"]; !exists {
			view["status"] = string(control.CommandStatusOK)
		}
	}
	return view, normalizeResilienceError(err)
}

func decodeActionInput(input map[string]interface{}, target interface{}) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return errors.WithCode(code.ErrInvalidArgument, "invalid input: %s", err.Error())
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return errors.WithCode(code.ErrInvalidArgument, "invalid input: %s", err.Error())
	}
	return nil
}

func actionActor(ctx context.Context, orgID int64) control.ActionActor {
	return control.ActionActor{OrgID: orgID, UserID: actorctx.GrantingUserID(ctx)}
}

func actionResultMap(value interface{}) map[string]interface{} {
	payload, _ := json.Marshal(value)
	result := map[string]interface{}{}
	_ = json.Unmarshal(payload, &result)
	return result
}

func normalizeResilienceError(err error) error {
	if err == nil {
		return nil
	}
	if stderrors.Is(err, control.ErrVersionConflict) {
		return errors.WithCode(code.ErrConflict, "%s", err.Error())
	}
	if stderrors.Is(err, control.ErrInvalidArgument) {
		return errors.WithCode(code.ErrInvalidArgument, "%s", err.Error())
	}
	if stderrors.Is(err, control.ErrUnavailable) {
		return errors.WithCode(code.ErrInternalServerError, "%s", err.Error())
	}
	return err
}

func (e *ActionExecutor) runReloadPolicy(ctx context.Context, orgID int64, input map[string]interface{}) (map[string]interface{}, error) {
	if e == nil || e.reloader == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "cache policy reloader unavailable")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "invalid input: %s", err.Error())
	}
	var req cachemodel.CachePolicyReloadRequest
	if err := json.Unmarshal(payload, &req); err != nil || req.ExpectedVersion == 0 {
		return nil, errors.WithCode(code.ErrInvalidArgument, "expected_version must be a positive integer")
	}
	req.ActorUserID = actorctx.GrantingUserID(ctx)
	result, err := e.reloader.ReloadPolicy(ctx, orgID, req)
	if err != nil {
		return nil, err
	}
	payload, _ = json.Marshal(result)
	view := map[string]interface{}{}
	_ = json.Unmarshal(payload, &view)
	return view, nil
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

func finalizeActionRun(requestID, actionID string, startedAt time.Time, result map[string]interface{}, err error) (*ActionRunResult, error) {
	if err != nil {
		return nil, err
	}
	status := string(control.CommandStatusOK)
	if projected, ok := result["status"].(string); ok {
		switch control.CommandStatus(projected) {
		case control.CommandStatusOK, control.CommandStatusNoop, control.CommandStatusPartial,
			control.CommandStatusTimeout, control.CommandStatusFailed:
			status = projected
		}
	}
	return &ActionRunResult{
		RequestID:  requestID,
		ActionID:   actionID,
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		Status:     status,
		Message:    fmt.Sprintf("action %s completed", actionID),
		Result:     result,
	}, nil
}
