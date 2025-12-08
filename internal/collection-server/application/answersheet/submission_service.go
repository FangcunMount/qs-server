package answersheet

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
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
	log.Infof("Submitting answer sheet: writerID=%d, testeeID=%d, questionnaireCode=%s",
		writerID, req.TesteeID, req.QuestionnaireCode)

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
		return nil, err
	}

	return &SubmitAnswerSheetResponse{
		ID:      result.ID,
		Message: result.Message,
	}, nil
}

// Get 获取答卷详情
func (s *SubmissionService) Get(ctx context.Context, id uint64) (*AnswerSheetResponse, error) {
	log.Infof("Getting answer sheet: id=%d", id)

	result, err := s.answerSheetClient.GetAnswerSheet(ctx, id)
	if err != nil {
		log.Errorf("Failed to get answer sheet via gRPC: %v", err)
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
