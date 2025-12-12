package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
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
		var data AnswerSheetSubmittedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse answersheet submitted event: %w", err)
		}

		deps.Logger.Info("processing answersheet submitted",
			slog.String("event_id", env.ID),
			slog.String("answersheet_id", data.AnswerSheetID),
			slog.String("questionnaire_code", data.QuestionnaireCode),
			slog.String("questionnaire_version", data.QuestionnaireVersion),
			slog.Uint64("filler_id", data.FillerID),
			slog.String("filler_type", data.FillerType),
			slog.Time("submitted_at", data.SubmittedAt),
		)

		// 检查 InternalClient 是否可用
		if deps.InternalClient == nil {
			deps.Logger.Warn("InternalClient is not available, skipping assessment creation",
				slog.String("answersheet_id", data.AnswerSheetID),
			)
			return nil
		}

		// 解析答卷 ID
		answerSheetID, err := strconv.ParseUint(data.AnswerSheetID, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid answersheet_id format: %w", err)
		}

		// 调用 InternalClient 创建 Assessment
		req := &pb.CreateAssessmentFromAnswerSheetRequest{
			AnswersheetId:        answerSheetID,
			QuestionnaireCode:    data.QuestionnaireCode,
			QuestionnaireVersion: data.QuestionnaireVersion,
			FillerId:             data.FillerID,
			FillerType:           data.FillerType,
			OriginType:           "adhoc", // 默认为即时测评
		}

		resp, err := deps.InternalClient.CreateAssessmentFromAnswerSheet(ctx, req)
		if err != nil {
			deps.Logger.Error("failed to create assessment from answersheet",
				slog.String("answersheet_id", data.AnswerSheetID),
				slog.String("error", err.Error()),
			)
			return fmt.Errorf("failed to create assessment: %w", err)
		}

		deps.Logger.Info("assessment created from answersheet",
			slog.String("answersheet_id", data.AnswerSheetID),
			slog.Uint64("assessment_id", resp.AssessmentId),
			slog.Bool("created", resp.Created),
			slog.Bool("auto_submitted", resp.AutoSubmitted),
			slog.String("message", resp.Message),
		)

		return nil
	}
}
