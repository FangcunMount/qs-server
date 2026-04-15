package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
)

func init() {
	// 注册答卷提交处理器
	Register("answersheet_submitted_handler", func(deps *Dependencies) HandlerFunc {
		return handleAnswerSheetSubmitted(deps)
	})
}

type answerSheetProcessingGateMode string

const (
	answerSheetProcessingGateModeLocked        answerSheetProcessingGateMode = "locked"
	answerSheetProcessingGateModeDuplicateSkip answerSheetProcessingGateMode = "duplicate_skip"
	answerSheetProcessingGateModeDegraded      answerSheetProcessingGateMode = "degraded"
)

type answerSheetProcessingGateHooks struct {
	acquire func(ctx context.Context, deps *Dependencies, answerSheetID uint64) (string, bool, error)
	release func(ctx context.Context, deps *Dependencies, answerSheetID uint64, token string) error
}

var defaultAnswerSheetProcessingGateHooks = answerSheetProcessingGateHooks{
	acquire: acquireProcessingLock,
	release: releaseProcessingLock,
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
	return func(ctx context.Context, eventType string, payload []byte) error {
		env, answerSheetID, data, err := parseAnswerSheetData(deps, payload)
		if err != nil {
			return fmt.Errorf("failed to parse answersheet submitted event: %w", err)
		}

		return withAnswerSheetProcessingGate(ctx, deps, env.ID, answerSheetID, hooks, func(runCtx context.Context) error {
			// Step 1: 计算答卷分数（在 Survey 域完成）
			if err := calculateAnswerSheetScore(runCtx, deps, answerSheetID); err != nil {
				deps.Logger.Error("failed to calculate answersheet score",
					slog.String("answersheet_id", strconv.FormatUint(answerSheetID, 10)),
					slog.String("error", err.Error()),
				)
				return fmt.Errorf("failed to calculate answersheet score: %w", err)
			}

			// Step 2: 创建 Assessment（在 Evaluation 域完成）
			if err := createAssessmentFromAnswerSheet(runCtx, deps, answerSheetID, data); err != nil {
				return fmt.Errorf("failed to create assessment from answersheet: %w", err)
			}

			return nil
		})
	}
}

// 解析答卷数据
func parseAnswerSheetData(deps *Dependencies, payload []byte) (*EventEnvelope, uint64, *domainAnswerSheet.AnswerSheetSubmittedData, error) {
	var data domainAnswerSheet.AnswerSheetSubmittedData
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

// withAnswerSheetProcessingGate 使用 best-effort Redis 闸门抑制重复工作。
// 真正正确性仍由下游幂等查询和数据库唯一索引保证。
func withAnswerSheetProcessingGate(
	ctx context.Context,
	deps *Dependencies,
	eventID string,
	answerSheetID uint64,
	hooks answerSheetProcessingGateHooks,
	fn func(context.Context) error,
) error {
	if hooks.acquire == nil {
		hooks.acquire = acquireProcessingLock
	}
	if hooks.release == nil {
		hooks.release = releaseProcessingLock
	}

	answerSheetIDStr := strconv.FormatUint(answerSheetID, 10)
	lockKey := answerSheetProcessingLockKey(answerSheetID)

	if deps.RedisCache == nil {
		deps.Logger.Warn("answersheet processing gate degraded",
			slog.String("event_id", eventID),
			slog.String("answersheet_id", answerSheetIDStr),
			slog.String("lock_key", lockKey),
			slog.String("lock_mode", string(answerSheetProcessingGateModeDegraded)),
			slog.String("reason", "redis_unavailable"),
		)
		return fn(ctx)
	}

	token, acquired, err := hooks.acquire(ctx, deps, answerSheetID)
	if err != nil {
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
		if err := hooks.release(ctx, deps, answerSheetID, token); err != nil {
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

// acquireProcessingLock 获取答卷处理的 best-effort Redis lease lock。
func acquireProcessingLock(ctx context.Context, deps *Dependencies, answerSheetID uint64) (string, bool, error) {
	if deps.RedisCache == nil {
		return "", false, nil
	}
	lockKey := answerSheetProcessingLockKey(answerSheetID)
	lockTTL := 5 * time.Minute

	token, acquired, err := redislock.Acquire(ctx, deps.RedisCache, lockKey, lockTTL)
	if err != nil {
		return "", false, fmt.Errorf("failed to acquire processing lock: %w", err)
	}
	return token, acquired, nil
}

// releaseProcessingLock 释放答卷处理的 Redis lease lock。
func releaseProcessingLock(ctx context.Context, deps *Dependencies, answerSheetID uint64, token string) error {
	if deps.RedisCache == nil {
		return nil
	}
	lockKey := answerSheetProcessingLockKey(answerSheetID)
	if err := redislock.Release(ctx, deps.RedisCache, lockKey, token); err != nil {
		return fmt.Errorf("failed to release processing lock: %w", err)
	}
	return nil
}

func answerSheetProcessingLockKey(answerSheetID uint64) string {
	return rediskey.NewBuilder().BuildAnswerSheetProcessingLockKey(answerSheetID)
}

// 计算答卷分数
func calculateAnswerSheetScore(ctx context.Context, deps *Dependencies, answerSheetID uint64) error {
	if deps.InternalClient == nil {
		return fmt.Errorf("internal client is not available")
	}
	scoreReq := &pb.CalculateAnswerSheetScoreRequest{
		AnswersheetId: answerSheetID,
	}
	scoreResp, err := deps.InternalClient.CalculateAnswerSheetScore(ctx, scoreReq)
	if err != nil {
		return fmt.Errorf("failed to calculate answersheet score: %w", err)
	}
	deps.Logger.Debug("answersheet scoring detail",
		"answersheet_id", strconv.FormatUint(answerSheetID, 10),
		"total_score", scoreResp.TotalScore,
		"message", scoreResp.Message,
	)
	return nil
}

// 创建测评
func createAssessmentFromAnswerSheet(ctx context.Context, deps *Dependencies, answerSheetID uint64, data *domainAnswerSheet.AnswerSheetSubmittedData) error {
	if deps.InternalClient == nil {
		return fmt.Errorf("internal client is not available")
	}
	// 构建创建测评请求
	assessmentReq := &pb.CreateAssessmentFromAnswerSheetRequest{
		AnswersheetId:        answerSheetID,
		QuestionnaireCode:    data.QuestionnaireCode,
		QuestionnaireVersion: data.QuestionnaireVersion,
		TesteeId:             data.TesteeID,
		OrgId:                data.OrgID,
		FillerId:             data.FillerID,
		FillerType:           data.FillerType,
		TaskId:               data.TaskID,
	}
	if data.TaskID == "" {
		assessmentReq.OriginType = "adhoc"
	}
	// 创建测评
	assessmentResp, err := deps.InternalClient.CreateAssessmentFromAnswerSheet(ctx, assessmentReq)
	if err != nil {
		return fmt.Errorf("failed to create assessment from answersheet: %w", err)
	}
	deps.Logger.Debug("assessment creation detail",
		"answersheet_id", strconv.FormatUint(answerSheetID, 10),
		"assessment_id", assessmentResp.AssessmentId,
		"created", assessmentResp.Created,
		"message", assessmentResp.Message,
	)
	return nil
}
