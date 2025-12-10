package answersheet

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
)

// SubmissionService 答卷提交服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 转换 REST DTO 到 gRPC 请求
// 2. 调用 apiserver 的 gRPC 服务
// 3. 转换 gRPC 响应到 REST DTO
type SubmissionService struct {
	answerSheetClient *grpcclient.AnswerSheetClient
}

// NewSubmissionService 创建答卷提交服务
func NewSubmissionService(
	answerSheetClient *grpcclient.AnswerSheetClient,
) *SubmissionService {
	return &SubmissionService{
		answerSheetClient: answerSheetClient,
	}
}

// Submit 提交答卷
// writerID 来自认证中间件解析的当前用户
func (s *SubmissionService) Submit(ctx context.Context, writerID uint64, req *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Submitting answer sheet: writerID=%d, testeeID=%d, questionnaireCode=%s",
		writerID, req.TesteeID, req.QuestionnaireCode)

	l.Infow("开始提交答卷",
		"action", "submit_answersheet",
		"writer_id", writerID,
		"testee_id", req.TesteeID,
		"questionnaire_code", req.QuestionnaireCode,
		"answer_count", len(req.Answers),
	)

	// 转换 answers
	answers := make([]grpcclient.AnswerInput, len(req.Answers))
	for i, a := range req.Answers {
		answers[i] = grpcclient.AnswerInput{
			QuestionCode: a.QuestionCode,
			QuestionType: a.QuestionType,
			Score:        a.Score,
			Value:        a.Value,
		}
	}

	// 调用 gRPC 服务
	l.Debugw("调用 gRPC 服务提交答卷",
		"questionnaire_code", req.QuestionnaireCode,
		"testee_id", req.TesteeID,
	)

	result, err := s.answerSheetClient.SaveAnswerSheet(ctx, &grpcclient.SaveAnswerSheetInput{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		Title:                req.Title,
		WriterID:             writerID,
		TesteeID:             req.TesteeID,
		Answers:              answers,
	})
	if err != nil {
		log.Errorf("Failed to save answer sheet via gRPC: %v", err)
		l.Errorw("提交答卷失败",
			"action", "submit_answersheet",
			"questionnaire_code", req.QuestionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	duration := time.Since(startTime)
	l.Infow("提交答卷成功",
		"action", "submit_answersheet",
		"result", "success",
		"answersheet_id", result.ID,
		"duration_ms", duration.Milliseconds(),
	)

	return &SubmitAnswerSheetResponse{
		ID:      result.ID,
		Message: result.Message,
	}, nil
}

// Get 获取答卷详情
func (s *SubmissionService) Get(ctx context.Context, id uint64) (*AnswerSheetResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Getting answer sheet: id=%d", id)

	l.Debugw("获取答卷详情",
		"action", "get_answersheet",
		"answersheet_id", id,
	)

	result, err := s.answerSheetClient.GetAnswerSheet(ctx, id)
	if err != nil {
		log.Errorf("Failed to get answer sheet via gRPC: %v", err)
		l.Errorw("获取答卷失败",
			"action", "get_answersheet",
			"answersheet_id", id,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	// 转换 answers
	answers := make([]Answer, len(result.Answers))
	for i, a := range result.Answers {
		answers[i] = Answer{
			QuestionCode: a.QuestionCode,
			QuestionType: a.QuestionType,
			Score:        a.Score,
			Value:        a.Value,
		}
	}

	duration := time.Since(startTime)
	l.Debugw("获取答卷成功",
		"action", "get_answersheet",
		"answersheet_id", id,
		"questionnaire_code", result.QuestionnaireCode,
		"answer_count", len(answers),
		"duration_ms", duration.Milliseconds(),
	)

	return &AnswerSheetResponse{
		ID:                   result.ID,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		Title:                result.Title,
		Score:                result.Score,
		WriterID:             result.WriterID,
		WriterName:           result.WriterName,
		TesteeID:             result.TesteeID,
		TesteeName:           result.TesteeName,
		Answers:              answers,
		CreatedAt:            result.CreatedAt,
		UpdatedAt:            result.UpdatedAt,
	}, nil
}
