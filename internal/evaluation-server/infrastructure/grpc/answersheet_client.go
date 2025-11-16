package grpc

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
	"github.com/FangcunMount/qs-server/pkg/log"
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

// SaveAnswerSheetScores 保存答卷答案和分数
func (c *AnswerSheetClient) SaveAnswerSheetScores(ctx context.Context, answerSheetID uint64, totalScore float64, answers []*answersheet.Answer) error {
	log.Infof("保存答卷答案和分数，答卷ID: %d, 总分: %d", answerSheetID, totalScore)

	// 调用 gRPC 服务
	resp, err := c.client.SaveAnswerSheetScores(ctx, &answersheet.SaveAnswerSheetScoresRequest{
		AnswerSheetId: answerSheetID,
		TotalScore:    totalScore,
		Answers:       answers,
	})
	if err != nil {
		return fmt.Errorf("保存答卷分数失败: %v", err)
	}

	log.Infof("答卷分数保存成功，答卷ID: %d, 总分: %d", resp.AnswerSheetId, resp.TotalScore)
	return nil
}
