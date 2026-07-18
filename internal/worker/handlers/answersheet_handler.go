package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"

	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"google.golang.org/grpc/metadata"
)

type answerSheetProcessingGateMode string

const (
	answerSheetProcessingGateModeLocked        answerSheetProcessingGateMode = "locked"
	answerSheetProcessingGateModeDuplicateSkip answerSheetProcessingGateMode = "duplicate_skip"
	answerSheetProcessingGateModeDegraded      answerSheetProcessingGateMode = "degraded"
)

type answerSheetProcessingGateHooks struct {
	acquire  func(ctx context.Context, deps *Dependencies, answerSheetID uint64) (*locklease.Lease, bool, error)
	release  func(ctx context.Context, deps *Dependencies, answerSheetID uint64, lease *locklease.Lease) error
	observer resilience.Observer
}

type DuplicateSuppressionGate interface {
	Run(ctx context.Context, deps *Dependencies, eventID string, answerSheetID uint64, fn func(context.Context) error) error
}

type answerSheetDuplicateSuppressionGate struct {
	hooks       answerSheetProcessingGateHooks
	manualHooks bool
	observer    resilience.Observer
}

var _ DuplicateSuppressionGate = answerSheetDuplicateSuppressionGate{}

func newAnswerSheetDuplicateSuppressionGate(hooks answerSheetProcessingGateHooks) DuplicateSuppressionGate {
	manualHooks := hooks.acquire != nil || hooks.release != nil
	if hooks.acquire == nil {
		hooks.acquire = acquireProcessingLock
	}
	if hooks.release == nil {
		hooks.release = releaseProcessingLock
	}
	return answerSheetDuplicateSuppressionGate{
		hooks:       hooks,
		manualHooks: manualHooks,
		observer:    defaultAnswerSheetGateObserver(hooks.observer),
	}
}

// handleAnswerSheetSubmitted 返回答卷提交处理函数
// 业务逻辑：
//  1. 解析答卷提交事件
//  2. 调用 InternalClient 确保 Assessment 存在
//  3. 对关联量表，内部服务创建后自动提交；若已存在但仍为 pending，
//     则本次 worker 重放负责幂等补提交并触发评估
func handleAnswerSheetSubmitted(deps *Dependencies) HandlerFunc {
	return handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{})
}

func handleAnswerSheetSubmittedWithHooks(
	deps *Dependencies,
	hooks answerSheetProcessingGateHooks,
) HandlerFunc {
	gate := newAnswerSheetDuplicateSuppressionGate(hooks)
	return func(ctx context.Context, _ string, payload []byte) error {
		env, answerSheetID, data, err := parseAnswerSheetData(deps, payload)
		if err != nil {
			return fmt.Errorf("failed to parse answersheet submitted event: %w", err)
		}

		return gate.Run(ctx, deps, env.ID, answerSheetID, func(runCtx context.Context) error {
			if data.RequestID != "" {
				runCtx = metadata.AppendToOutgoingContext(runCtx, "x-request-id", data.RequestID)
			}
			if err := createAssessmentFromAnswerSheet(runCtx, deps, answerSheetID, data); err != nil {
				deps.Logger.Error("answersheet assessment ensure failed",
					"action", "answersheet_submitted", "stage", "ensure_assessment", "result", "failed",
					"error_category", "assessment_intake", "request_id", data.RequestID,
					"event_id", env.ID, "answersheet_id", data.AnswerSheetID, "error", err.Error(),
				)
				return fmt.Errorf("failed to create assessment from answersheet: %w", err)
			}
			deps.Logger.Info("answersheet submitted event processed",
				"action", "answersheet_submitted", "stage", "ensure_assessment", "result", "succeeded",
				"request_id", data.RequestID, "event_id", env.ID, "answersheet_id", data.AnswerSheetID,
			)
			return nil
		})
	}
}

// 解析答卷数据
func parseAnswerSheetData(deps *Dependencies, payload []byte) (*EventEnvelope, uint64, *eventpayload.AnswerSheetSubmittedData, error) {
	var data eventpayload.AnswerSheetSubmittedData
	env, err := ParseEventData(payload, &data)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to parse answersheet submitted event: %w", err)
	}

	// 解析答卷 ID
	answerSheetID, err := strconv.ParseUint(data.AnswerSheetID, 10, 64)
	if err != nil || answerSheetID == 0 {
		return nil, 0, nil, fmt.Errorf("invalid answersheet_id format or value: %w", err)
	}

	deps.Logger.Info("received answersheet submitted event",
		"action", "answersheet_submitted",
		"stage", "event_received",
		"result", "accepted",
		"event_id", env.ID,
		"answersheet_id", data.AnswerSheetID,
		"questionnaire_code", data.QuestionnaireCode,
		"questionnaire_version", data.QuestionnaireVersion,
		"testee_id", data.TesteeID,
		"org_id", data.OrgID,
		"filler_id", data.FillerID,
		"filler_type", data.FillerType,
		"task_id", data.TaskID,
		"request_id", data.RequestID,
		"submitted_at", data.SubmittedAt,
	)
	return env, answerSheetID, &data, nil
}

func (g answerSheetDuplicateSuppressionGate) Run(
	ctx context.Context,
	deps *Dependencies,
	eventID string,
	answerSheetID uint64,
	fn func(context.Context) error,
) error {
	answerSheetIDStr := strconv.FormatUint(answerSheetID, 10)
	lockKey := answerSheetProcessingLockKey(deps, answerSheetID)
	if deps.LockRunner != nil && !g.manualHooks {
		result, err := deps.LockRunner.Run(
			ctx,
			locklease.WorkloadAnswersheetProcessing,
			answerSheetProcessingLockKeyBase(answerSheetID),
			0,
			fn,
		)
		if errors.Is(err, locklease.ErrLeaseAcquireFailed) {
			observability.ObserveLockDegraded("answersheet_processing", "acquire_failed")
			g.observe(ctx, resilience.OutcomeDegradedOpen)
			deps.Logger.Warn("answersheet processing gate degraded",
				slog.String("event_id", eventID),
				slog.String("answersheet_id", answerSheetIDStr),
				slog.String("lock_key", lockKey),
				slog.String("lock_mode", string(answerSheetProcessingGateModeDegraded)),
				slog.String("reason", "acquire_failed"),
				slog.String("error", err.Error()),
			)
			return fn(ctx)
		}
		if err != nil {
			return err
		}
		if !result.Acquired {
			g.observe(ctx, resilience.OutcomeDuplicateSkipped)
			deps.Logger.Info("answersheet processing skipped as duplicate",
				slog.String("event_id", eventID),
				slog.String("answersheet_id", answerSheetIDStr),
				slog.String("lock_key", lockKey),
				slog.String("lock_mode", string(answerSheetProcessingGateModeDuplicateSkip)),
			)
			return nil
		}
		if result.ReleaseErr != nil {
			deps.Logger.Warn("failed to release answersheet processing gate",
				slog.String("event_id", eventID),
				slog.String("answersheet_id", answerSheetIDStr),
				slog.String("lock_key", lockKey),
				slog.String("error", result.ReleaseErr.Error()),
			)
		}
		return nil
	}

	if !lockManagerAvailable(deps.LockManager) {
		observability.ObserveLockDegraded("answersheet_processing", "redis_unavailable")
		g.observe(ctx, resilience.OutcomeDegradedOpen)
		deps.Logger.Warn("answersheet processing gate degraded",
			slog.String("event_id", eventID),
			slog.String("answersheet_id", answerSheetIDStr),
			slog.String("lock_key", lockKey),
			slog.String("lock_mode", string(answerSheetProcessingGateModeDegraded)),
			slog.String("reason", "redis_unavailable"),
		)
		return fn(ctx)
	}

	lease, acquired, err := g.hooks.acquire(ctx, deps, answerSheetID)
	if err != nil {
		observability.ObserveLockDegraded("answersheet_processing", "acquire_failed")
		g.observe(ctx, resilience.OutcomeDegradedOpen)
		deps.Logger.Warn("answersheet processing gate degraded",
			slog.String("event_id", eventID),
			slog.String("answersheet_id", answerSheetIDStr),
			slog.String("lock_key", lockKey),
			slog.String("lock_mode", string(answerSheetProcessingGateModeDegraded)),
			slog.String("reason", "acquire_failed"),
			slog.String("error", err.Error()),
		)
		return fn(ctx)
	}
	if !acquired {
		g.observe(ctx, resilience.OutcomeDuplicateSkipped)
		deps.Logger.Info("answersheet processing skipped as duplicate",
			slog.String("event_id", eventID),
			slog.String("answersheet_id", answerSheetIDStr),
			slog.String("lock_key", lockKey),
			slog.String("lock_mode", string(answerSheetProcessingGateModeDuplicateSkip)),
		)
		return nil
	}

	deps.Logger.Debug("answersheet processing gate acquired",
		slog.String("event_id", eventID),
		slog.String("answersheet_id", answerSheetIDStr),
		slog.String("lock_key", lockKey),
		slog.String("lock_mode", string(answerSheetProcessingGateModeLocked)),
	)

	defer func() {
		if err := g.hooks.release(ctx, deps, answerSheetID, lease); err != nil {
			deps.Logger.Warn("failed to release answersheet processing gate",
				slog.String("event_id", eventID),
				slog.String("answersheet_id", answerSheetIDStr),
				slog.String("lock_key", lockKey),
				slog.String("lock_mode", string(answerSheetProcessingGateModeLocked)),
				slog.String("error", err.Error()),
			)
		}
	}()

	return fn(ctx)
}

func (g answerSheetDuplicateSuppressionGate) observe(ctx context.Context, outcome resilience.Outcome) {
	resilience.Observe(ctx, g.observer, resilience.ProtectionDuplicateSuppression, resilience.Subject{
		Component: "worker",
		Scope:     "answersheet_submitted",
		Resource:  "answersheet_processing",
		Strategy:  "redis_lock",
	}, outcome)
}

func defaultAnswerSheetGateObserver(observer resilience.Observer) resilience.Observer {
	if observer != nil {
		return observer
	}
	return resilience.DefaultObserver()
}

// acquireProcessingLock 获取答卷处理的 best-effort Redis lease lock。
func acquireProcessingLock(ctx context.Context, deps *Dependencies, answerSheetID uint64) (*locklease.Lease, bool, error) {
	if !lockManagerAvailable(deps.LockManager) {
		return nil, false, fmt.Errorf("lock manager is unavailable")
	}
	capability, _ := locklease.Lookup(locklease.WorkloadAnswersheetProcessing)
	lease, acquired, err := deps.LockManager.AcquireSpec(ctx, capability.Spec, answerSheetProcessingLockKeyBase(answerSheetID))
	if err != nil {
		return nil, false, fmt.Errorf("failed to acquire processing lock: %w", err)
	}
	return lease, acquired, nil
}

// releaseProcessingLock 释放答卷处理的 Redis lease lock。
func releaseProcessingLock(ctx context.Context, deps *Dependencies, answerSheetID uint64, lease *locklease.Lease) error {
	if !lockManagerAvailable(deps.LockManager) {
		return nil
	}
	capability, _ := locklease.Lookup(locklease.WorkloadAnswersheetProcessing)
	if err := deps.LockManager.ReleaseSpec(ctx, capability.Spec, answerSheetProcessingLockKeyBase(answerSheetID), lease); err != nil {
		return fmt.Errorf("failed to release processing lock: %w", err)
	}
	return nil
}

func lockManagerAvailable(manager locklease.Manager) bool {
	if manager == nil {
		return false
	}
	value := reflect.ValueOf(manager)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

func answerSheetProcessingLockKeyBase(answerSheetID uint64) string {
	return "answersheet:processing:" + strconv.FormatUint(answerSheetID, 10)
}

func answerSheetProcessingLockKey(deps *Dependencies, answerSheetID uint64) string {
	if deps != nil && deps.LockKeyBuilder != nil {
		return deps.LockKeyBuilder.BuildAnswerSheetProcessingLockKey(answerSheetID)
	}
	return answerSheetProcessingLockKeyBase(answerSheetID)
}

// 创建测评
func createAssessmentFromAnswerSheet(ctx context.Context, deps *Dependencies, answerSheetID uint64, data *eventpayload.AnswerSheetSubmittedData) error {
	if deps.AssessmentIntakeClient == nil {
		return fmt.Errorf("assessment intake client is not available")
	}
	// 构建创建测评请求
	assessmentReq := &evalpb.EnsureAssessmentRequest{
		AnswerSheetId:        answerSheetID,
		QuestionnaireCode:    data.QuestionnaireCode,
		QuestionnaireVersion: data.QuestionnaireVersion,
		TesteeId:             data.TesteeID,
		OrgId:                data.OrgID,
		FillerId:             data.FillerID,
		TaskId:               data.TaskID,
	}
	if data.TaskID == "" {
		assessmentReq.OriginType = "adhoc"
	}
	// 创建测评
	assessmentResp, err := deps.AssessmentIntakeClient.EnsureAssessment(ctx, assessmentReq)
	if err != nil {
		return fmt.Errorf("failed to create assessment from answersheet: %w", err)
	}
	if assessmentResp == nil {
		return fmt.Errorf("assessment creation failed: empty response")
	}
	deps.Logger.Info("assessment ensured from answersheet event",
		"request_id", data.RequestID,
		"answersheet_id", strconv.FormatUint(answerSheetID, 10),
		"assessment_id", assessmentResp.AssessmentId,
		"created", assessmentResp.Created,
	)
	return nil
}
