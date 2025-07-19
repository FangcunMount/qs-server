package grpc

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/questionnaire"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// QuestionnaireClient 问卷客户端
type QuestionnaireClient struct {
	client questionnaire.QuestionnaireServiceClient
}

// NewQuestionnaireClient 创建问卷客户端
func NewQuestionnaireClient(factory *ClientFactory) *QuestionnaireClient {
	return &QuestionnaireClient{
		client: factory.NewQuestionnaireClient(),
	}
}

// GetQuestionnaire 根据问卷代码获取问卷详情
func (c *QuestionnaireClient) GetQuestionnaire(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	log.Infof("获取问卷详情，代码: %s", code)

	// 调用 gRPC 服务
	resp, err := c.client.GetQuestionnaire(ctx, &questionnaire.GetQuestionnaireRequest{
		Code: code,
	})
	if err != nil {
		return nil, fmt.Errorf("获取问卷详情失败: %v", err)
	}

	return resp.Questionnaire, nil
}
