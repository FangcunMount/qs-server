package grpc

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// AnswerSheetClient 答卷客户端
type AnswerSheetClient struct {
	client answersheet.AnswerSheetServiceClient
}

// NewAnswerSheetClient 创建答卷客户端
func NewAnswerSheetClient(factory *ClientFactory) *AnswerSheetClient {
	return &AnswerSheetClient{
		client: factory.NewAnswerSheetClient(),
	}
}

// GetAnswerSheet 根据答卷ID获取答卷详情
func (c *AnswerSheetClient) GetAnswerSheet(ctx context.Context, id uint64) (*answersheet.AnswerSheet, error) {
	log.Infof("获取答卷详情，ID: %d", id)

	// 调用 gRPC 服务
	resp, err := c.client.GetAnswerSheet(ctx, &answersheet.GetAnswerSheetRequest{
		Id: id,
	})
	if err != nil {
		return nil, fmt.Errorf("获取答卷详情失败: %v", err)
	}

	return resp.AnswerSheet, nil
}
