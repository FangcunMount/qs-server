package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	"github.com/FangcunMount/qs-server/internal/worker/infra/redislock"
)

func init() {
	// 注册答卷提交处理器
	Register("answersheet_submitted_handler", func(deps *Dependencies) HandlerFunc {
		return handleAnswerSheetSubmitted(deps)
	})
}

// ==================== Payload 定义 ====================

// AnswerSheetSubmittedPayload 答卷提交事件数据
// 对应发布端 answersheet.AnswerSheetSubmittedData
type AnswerSheetSubmittedPayload struct {
	AnswerSheetID        string    `json:"answersheet_id"`
	QuestionnaireCode    string    `json:"questionnaire_code"`
	QuestionnaireVersion string    `json:"questionnaire_version"`
	TesteeID             uint64    `json:"testee_id"` // 受试者ID
	OrgID                uint64    `json:"org_id"`    // 组织ID
	FillerID             uint64    `json:"filler_id"`
	FillerType           string    `json:"filler_type"`
	SubmittedAt          time.Time `json:"submitted_at"`
}

// ==================== Handler 实现 ====================

// handleAnswerSheetSubmitted 返回答卷提交处理函数
// 业务逻辑：
// 1. 解析答卷提交事件
// 2. 调用 InternalClient 创建 Assessment
// 3. 如果关联量表，Assessment 会自动提交并触发评估
func handleAnswerSheetSubmitted(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		answerSheetID, data, err := parseAnswerSheetData(deps, payload)
		if err != nil {
			return fmt.Errorf("failed to parse answersheet submitted event: %w", err)
		}

		// 分布式锁，防止同一答卷并发/重放创建测评
		token, _, err := acquireProcessingLock(ctx, deps, answerSheetID)
		if err != nil {
			return fmt.Errorf("failed to acquire processing lock: %w", err)
		}
		defer func() {
			if err := releaseProcessingLock(ctx, deps, answerSheetID, token); err != nil {
				deps.Logger.Warn("failed to release processing lock",
					slog.String("answersheet_id", strconv.FormatUint(answerSheetID, 10)),
					slog.String("error", err.Error()),
				)
			}
		}()

		// Step 1: 计算答卷分数（在 Survey 域完成）
		if err := calculateAnswerSheetScore(ctx, deps, answerSheetID); err != nil {
			deps.Logger.Error("failed to calculate answersheet score",
				slog.String("answersheet_id", strconv.FormatUint(answerSheetID, 10)),
				slog.String("error", err.Error()),
			)
			return fmt.Errorf("failed to calculate answersheet score: %w", err)
		}

		// Step 2: 创建 Assessment（在 Evaluation 域完成）
		if err := createAssessmentFromAnswerSheet(ctx, deps, answerSheetID, data); err != nil {
			return fmt.Errorf("failed to create assessment from answersheet: %w", err)
		}

		return nil
	}
}

// 解析答卷数据
func parseAnswerSheetData(deps *Dependencies, payload []byte) (uint64, *AnswerSheetSubmittedPayload, error) {
	var data AnswerSheetSubmittedPayload
	env, err := ParseEventData(payload, &data)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to parse answersheet submitted event: %w", err)
	}

	// 解析答卷 ID
	answerSheetID, err := strconv.ParseUint(data.AnswerSheetID, 10, 64)
	if err != nil || answerSheetID == 0 {
		return 0, nil, fmt.Errorf("invalid answersheet_id format or value: %w", err)
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
		"submitted_at", data.SubmittedAt,
	)
	return answerSheetID, &data, nil
}

// 分布式锁，防止同一答卷并发/重放创建测评
func acquireProcessingLock(ctx context.Context, deps *Dependencies, answerSheetID uint64) (string, bool, error) {
	if deps.RedisCache == nil {
		return "", false, nil
	}
	// 锁的key为 answersheet:processing:${answerSheetID}
	lockKey := fmt.Sprintf("answersheet:processing:%d", answerSheetID)
	lockTTL := 5 * time.Minute

	// 获取分布式锁
	token, acquired, err := redislock.Acquire(ctx, deps.RedisCache, lockKey, lockTTL)
	if err != nil {
		return "", false, fmt.Errorf("failed to acquire processing lock: %w", err)
	}
	// 如果获取失败，则返回false
	if !acquired {
		deps.Logger.Info("skip duplicated answersheet processing",
			slog.String("answersheet_id", strconv.FormatUint(answerSheetID, 10)),
		)
		return "", false, nil
	}

	deps.Logger.Debug("acquired processing lock",
		"answersheet_id", strconv.FormatUint(answerSheetID, 10),
		"token", token,
	)
	return token, true, nil
}

// 释放分布式锁
func releaseProcessingLock(ctx context.Context, deps *Dependencies, answerSheetID uint64, token string) error {
	if deps.RedisCache == nil {
		return nil
	}
	lockKey := fmt.Sprintf("answersheet:processing:%d", answerSheetID)
	if err := redislock.Release(ctx, deps.RedisCache, lockKey, token); err != nil {
		return fmt.Errorf("failed to release processing lock: %w", err)
	}
	return nil
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
func createAssessmentFromAnswerSheet(ctx context.Context, deps *Dependencies, answerSheetID uint64, data *AnswerSheetSubmittedPayload) error {
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
		OriginType:           "adhoc",
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
