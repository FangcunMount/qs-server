package answersheet

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
)

// SubmissionCommitter 调用 gRPC 保存答卷。
type SubmissionCommitter struct {
	gateway answerSheetGateway
}

func NewSubmissionCommitter(gateway answerSheetGateway) *SubmissionCommitter {
	return &SubmissionCommitter{gateway: gateway}
}

func (c *SubmissionCommitter) Save(
	ctx context.Context,
	writerID, orgID, testeeID uint64,
	req *SubmitAnswerSheetRequest,
	answers []grpcbridge.AnswerInput,
) (*grpcbridge.SaveAnswerSheetOutput, error) {
	if c == nil || c.gateway == nil {
		return nil, nil
	}
	l := logger.L(ctx)

	l.Debugw("调用 gRPC 服务提交答卷",
		"questionnaire_code", req.QuestionnaireCode,
		"testee_id", testeeID,
		"org_id", orgID,
	)

	result, err := c.gateway.SaveAnswerSheet(ctx, &grpcbridge.SaveAnswerSheetInput{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		IdempotencyKey:       req.IdempotencyKey,
		Title:                req.Title,
		WriterID:             writerID,
		TesteeID:             testeeID,
		TaskID:               req.TaskID,
		OrgID:                orgID,
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
	return result, nil
}
