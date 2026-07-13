package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"

	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
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
	observer resilienceplane.Observer
}

type DuplicateSuppressionGate interface {
	Run(ctx context.Context, deps *Dependencies, eventID string, answerSheetID uint64, fn func(context.Context) error) error
}

type answerSheetDuplicateSuppressionGate struct {
	hooks    answerSheetProcessingGateHooks
	observer resilienceplane.Observer
}

var _ DuplicateSuppressionGate = answerSheetDuplicateSuppressionGate{}

var defaultAnswerSheetProcessingGateHooks = answerSheetProcessingGateHooks{
	acquire: acquireProcessingLock,
	release: releaseProcessingLock,
}

func newAnswerSheetDuplicateSuppressionGate(hooks answerSheetProcessingGateHooks) DuplicateSuppressionGate {
	if hooks.acquire == nil {
		hooks.acquire = acquireProcessingLock
	}
	if hooks.release == nil {
		hooks.release = releaseProcessingLock
	}
	return answerSheetDuplicateSuppressionGate{
		hooks:    hooks,
		observer: defaultAnswerSheetGateObserver(hooks.observer),
	}
}

// handleAnswerSheetSubmitted 返回答卷提交处理函数
// 业务逻辑：
// 1. 解析答卷提交事件
// 2. 调用 InternalClient 创建 Assessment
// 3. 创建 Assessment；如果关联量表，由内部服务显式提交并触发评估
func handleAnswerSheetSubmitted(deps *Dependencies) HandlerFunc {
	return handleAnswerSheetSubmittedWithHooks(deps, defaultAnswerSheetProcessingGateHooks)
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
			if err := createAssessmentFromAnswerSheet(runCtx, deps, answerSheetID, data); err != nil {
				return fmt.Errorf("failed to create assessment from answersheet: %w", err)
			}
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

	deps.Logger.Debug("answersheet submitted detail",
		"event_id", env.ID,
		"answersheet_id", data.AnswerSheetID,
		"questionnaire_code", data.QuestionnaireCode,
		"questionnaire_version", data.QuestionnaireVersion,
		"testee_id", data.TesteeID,
		"org_id", data.OrgID,
		"filler_id", data.FillerID,
		"filler_type", data.FillerType,
		"task_id", data.TaskID,
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

	if !lockManagerAvailable(deps.LockManager) {
		observability.ObserveLockDegraded("answersheet_processing", "redis_unavailable")
		g.observe(ctx, resilienceplane.OutcomeDegradedOpen)
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
		g.observe(ctx, resilienceplane.OutcomeDegradedOpen)
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
		g.observe(ctx, resilienceplane.OutcomeDuplicateSkipped)
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

func (g answerSheetDuplicateSuppressionGate) observe(ctx context.Context, outcome resilienceplane.Outcome) {
	resilienceplane.Observe(ctx, g.observer, resilienceplane.ProtectionDuplicateSuppression, resilienceplane.Subject{
		Component: "worker",
		Scope:     "answersheet_submitted",
		Resource:  "answersheet_processing",
		Strategy:  "redis_lock",
	}, outcome)
}

func defaultAnswerSheetGateObserver(observer resilienceplane.Observer) resilienceplane.Observer {
	if observer != nil {
		return observer
	}
	return resilienceplane.DefaultObserver()
}

// acquireProcessingLock 获取答卷处理的 best-effort Redis lease lock。
func acquireProcessingLock(ctx context.Context, deps *Dependencies, answerSheetID uint64) (*locklease.Lease, bool, error) {
	if !lockManagerAvailable(deps.LockManager) {
		return nil, false, fmt.Errorf("lock manager is unavailable")
	}
	lease, acquired, err := deps.LockManager.AcquireSpec(ctx, locklease.Specs.AnswersheetProcessing, answerSheetProcessingLockKeyBase(answerSheetID))
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
	if err := deps.LockManager.ReleaseSpec(ctx, locklease.Specs.AnswersheetProcessing, answerSheetProcessingLockKeyBase(answerSheetID), lease); err != nil {
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
	deps.Logger.Debug("assessment creation detail",
		"answersheet_id", strconv.FormatUint(answerSheetID, 10),
		"assessment_id", assessmentResp.AssessmentId,
		"created", assessmentResp.Created,
	)
	return nil
}
